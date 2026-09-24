#!/usr/bin/env python3
"""greedy.guru bench matrix — collect `greedy bench` runs into site/bench-matrix.json.

SSOT is the JSON in this repo: the landing reads it, the Google Sheet
("greedy-guru-benchmarks") is synced FROM it — never the other way.

Usage:
  python3 scripts/bench-matrix.py run --env mac --label "Mac" \
      --note "5 Chrome.app headless" -- \
      go run ./cmd/greedy bench --base-url URL --seq 1,5,10,25 \
      --parallel 5 --waves 2,5 --repeat 10 --mode none CRYSTAL

  python3 scripts/bench-matrix.py table [--json site/bench-matrix.json]
  python3 scripts/bench-matrix.py sheet-values  # rows for google-sheets.py update
"""
from __future__ import annotations

import argparse
import datetime as dt
import json
import os
import subprocess
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
DEFAULT_JSON = ROOT / "site" / "bench-matrix.json"

# Cell key kind order for column sorting.
KIND_ORDER = {"seq": 0, "par": 1, "waves": 2}


def load_matrix(path: Path) -> dict:
    if path.is_file():
        return json.loads(path.read_text(encoding="utf-8"))
    return {
        "clock": "",
        "crystal": "",
        "base_url": "",
        "repeat": 0,
        "reset": "",
        "envs": [],
        "cells": {},
    }


def column_label(key: str, parallel: int) -> str:
    kind, _, n = key.partition(":")
    n = int(n)
    if kind == "seq":
        if n == 1:
            return "1 тест"
        return f"{n} тестов в 1 контейнере"
    if kind == "par":
        return f"{n} тестов по 1 в контейнере, {n} контейнеров"
    if kind == "waves":
        total = n * parallel
        return f"{total} тестов в {parallel} контейнерах, по {n} в одном"
    return key


def columns_of(matrix: dict) -> list[str]:
    keys: set[str] = set()
    for cells in matrix.get("cells", {}).values():
        keys.update(cells.keys())
    return sorted(
        keys, key=lambda k: (KIND_ORDER.get(k.split(":")[0], 9), int(k.split(":")[1]))
    )


def cmd_run(args: argparse.Namespace) -> int:
    if not args.cmd:
        print("bench-matrix run: pass the bench command after --", file=sys.stderr)
        return 2
    proc = subprocess.run(args.cmd, cwd=ROOT, capture_output=True, text=True)
    raw = proc.stdout.strip()
    try:
        rep = json.loads(raw)
    except json.JSONDecodeError:
        sys.stderr.write(proc.stderr)
        print(f"bench-matrix run: command did not emit JSON (exit {proc.returncode})", file=sys.stderr)
        print(raw[:2000], file=sys.stderr)
        return 1
    if not rep.get("ok"):
        sys.stderr.write(proc.stderr)
        print(f"bench-matrix run: bench failed: {rep.get('error', '?')}", file=sys.stderr)
        print(raw[:2000], file=sys.stderr)
        return 1

    cells: dict[str, dict] = {}
    for cell in rep.get("seq") or []:
        cells[f"seq:{cell['n']}"] = cell
    if rep.get("par"):
        cells[f"par:{rep['par']['n']}"] = rep["par"]
    for cell in rep.get("waves") or []:
        cells[f"waves:{cell['n']}"] = cell
    if not cells:
        print("bench-matrix run: report has no cells", file=sys.stderr)
        return 1

    matrix = load_matrix(Path(args.json))
    matrix["measured_at"] = dt.date.today().isoformat()
    matrix["clock"] = rep.get("clock", matrix.get("clock", ""))
    matrix["reset"] = rep.get("reset", matrix.get("reset", ""))
    matrix["repeat"] = rep.get("repeat", matrix.get("repeat", 0))
    if args.crystal:
        matrix["crystal"] = args.crystal
    if args.base_url:
        matrix["base_url"] = args.base_url

    envs = [e for e in matrix.get("envs", []) if e.get("id") != args.env]
    envs.append(
        {
            "id": args.env,
            "label": args.label or args.env,
            "note": args.note or "",
            "parallel": args.parallel or 0,
        }
    )
    matrix["envs"] = envs
    matrix.setdefault("cells", {})[args.env] = cells

    out = Path(args.json)
    out.parent.mkdir(parents=True, exist_ok=True)
    out.write_text(json.dumps(matrix, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
    print(json.dumps({"ok": True, "env": args.env, "cells": sorted(cells), "json": str(out)}))
    return 0


def median_row(matrix: dict, env: dict, cols: list[str], field: str) -> list:
    cells = matrix.get("cells", {}).get(env["id"], {})
    return [cells.get(c, {}).get(field, "") for c in cols]


def sheet_rows(matrix: dict) -> list[list]:
    cols = columns_of(matrix)
    envs = matrix.get("envs", [])
    par_by_env = {e["id"]: int(e.get("parallel") or 0) for e in envs}
    repeat = int(matrix.get("repeat") or 0)

    head = ["стена, ms", ""] + [column_label(c, max(par_by_env.values() or [0])) for c in cols]
    rows = [head]
    for env in envs:
        label = env.get("label") or env["id"]
        rows.append(["", f"{label}, медиана {repeat}"] + median_row(matrix, env, cols, "median_ms"))
        rows.append(["", f"{label}, лучший из {repeat}"] + median_row(matrix, env, cols, "best_ms"))
    return rows


def sheet_footer(matrix: dict) -> list[list]:
    return [
        [],
        ["Лист", "Лучший = min из засчитанных. Медиана рядом: пропорции читать по ней, не по min разных клеток."],
        ["Протокол", f"greedy bench --repeat {matrix.get('repeat', '?')}. Warmup команды + первый прогон клетки выкинуты. par/waves до очереди. GREEDY_RESET={matrix.get('reset', 'on')}. Неудачный sample ретраится до 3 раз, в историю не пишется."],
        ["Дата", f"{matrix.get('measured_at', '?')}. {matrix.get('crystal', '')} · {matrix.get('base_url', '')} · --repeat {matrix.get('repeat', '?')}"],
        ["История", "вкладка История: все прогоны каждой клетки (runs_ms)."],
        ["Не путать", "Пин лендинга = testdata/app-live. Jenkins Test = 1× POST /run."],
    ]


def cmd_table(args: argparse.Namespace) -> int:
    matrix = load_matrix(Path(args.json))
    rows = sheet_rows(matrix) + sheet_footer(matrix)
    width = max(len(r) for r in rows)
    for r in rows:
        print(" | ".join(str(c) for c in r + [""] * (width - len(r))))
    return 0


def cmd_sheet_values(args: argparse.Namespace) -> int:
    matrix = load_matrix(Path(args.json))
    out = {
        "sheet1": sheet_rows(matrix) + sheet_footer(matrix),
        "history": history_rows(matrix),
    }
    print(json.dumps(out, ensure_ascii=False))
    return 0


def _pad(rows: list[list], width: int, height: int) -> list[list]:
    """Pad with blanks so `update` clears stale cells below/right of new data."""
    out = [list(r) + [""] * (width - len(r)) for r in rows]
    while len(out) < height:
        out.append([""] * width)
    return out


def _find_sheets_script() -> Path | None:
    env = os.environ.get("GREEDY_SHEETS_SCRIPT", "").strip()
    if env:
        p = Path(env)
        return p if p.is_file() else None
    # workspace checkout: <ws>/projects/greedy-guru-home/greedy-guru → <ws>/scripts/...
    ws = ROOT.parents[2]
    p = ws / "scripts" / "google-sheets" / "google-sheets.py"
    return p if p.is_file() else None


def cmd_sync_sheets(args: argparse.Namespace) -> int:
    matrix = load_matrix(Path(args.json))
    script = _find_sheets_script()
    if script is None:
        print(
            "bench-matrix sync-sheets: google-sheets.py not found "
            "(set GREEDY_SHEETS_SCRIPT or run inside the zds workspace)",
            file=sys.stderr,
        )
        return 1
    cols = columns_of(matrix)
    width = max(len(cols) + 2, 10)
    jobs = [
        ("Sheet1", _pad(sheet_rows(matrix) + sheet_footer(matrix), width, 30)),
        ("История", _pad(history_rows(matrix), 4, 600)),
    ]
    results = []
    for tab, values in jobs:
        rng = f"'{tab}'!A1"
        cmd = [
            sys.executable, str(script), "update",
            "--alias", args.alias,
            "--range", rng,
            "--values-json", json.dumps(values, ensure_ascii=False),
        ]
        if args.dry_run:
            cmd.append("--dry-run")
        proc = subprocess.run(cmd, capture_output=True, text=True)
        out = proc.stdout.strip()
        try:
            payload = json.loads(out)
        except json.JSONDecodeError:
            payload = {"ok": False, "raw": out[:500]}
        payload["tab"] = tab
        if proc.returncode != 0 or not payload.get("ok"):
            payload["stderr"] = proc.stderr.strip()[:500]
            print(json.dumps({"ok": False, "results": results + [payload]}, ensure_ascii=False))
            return 1
        results.append(payload)
    print(json.dumps({"ok": True, "dry_run": args.dry_run, "results": results}, ensure_ascii=False))
    return 0


def history_rows(matrix: dict) -> list[list]:
    cols = columns_of(matrix)
    rows = [["env", "column", "run_i", "wall_ms"]]
    for env in matrix.get("envs", []):
        cells = matrix.get("cells", {}).get(env["id"], {})
        for c in cols:
            runs = cells.get(c, {}).get("runs_ms") or []
            for i, ms in enumerate(runs, 1):
                rows.append([env.get("label") or env["id"], c, i, ms])
    return rows


def main() -> int:
    p = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    p.add_argument("--json", default=str(DEFAULT_JSON), help="matrix JSON path (default site/bench-matrix.json)")
    sub = p.add_subparsers(dest="action", required=True)

    pr = sub.add_parser("run", help="run a `greedy bench` command and merge cells into the matrix")
    pr.add_argument("--env", required=True, help="environment id, e.g. mac | selenoid")
    pr.add_argument("--label", default="", help="display label, e.g. Mac / selenoid.qa.guru")
    pr.add_argument("--note", default="", help="env note, e.g. '5 Chrome.app headless'")
    pr.add_argument("--parallel", type=int, default=0, help="N containers for par/waves labels")
    pr.add_argument("--crystal", default="", help="crystal path for metadata")
    pr.add_argument("--base-url", default="", help="base URL for metadata")
    pr.add_argument("cmd", nargs=argparse.REMAINDER, help="bench command after --")

    sub.add_parser("table", help="print the sheet-style table")
    sub.add_parser("sheet-values", help="emit JSON rows for google-sheets.py update")

    ps = sub.add_parser("sync-sheets", help="push the matrix into the Google Sheet via google-sheets.py")
    ps.add_argument("--alias", default="greedy-guru-benchmarks", help="registry alias")
    ps.add_argument("--dry-run", action="store_true", help="print planned updates, no writes")

    args = p.parse_args()
    if args.action == "run":
        if args.cmd and args.cmd[0] == "--":
            args.cmd = args.cmd[1:]
        return cmd_run(args)
    if args.action == "table":
        return cmd_table(args)
    if args.action == "sheet-values":
        return cmd_sheet_values(args)
    if args.action == "sync-sheets":
        return cmd_sync_sheets(args)
    return 2


if __name__ == "__main__":
    raise SystemExit(main())
