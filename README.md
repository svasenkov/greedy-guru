# greedy

**Write tests in Playwright. Replay them as crystals on hot Chrome — a single Go binary, raw CDP, no Node.**

[RU version → README-RU.md](README-RU.md) · Site: [greedy.guru](https://greedy.guru)

```
Playwright test → green run trace.zip → greedy crystallize → login.json (IR)
                                                             ↓
TestOps approve                                    greedy run → hot Chrome (CDP)
```

A crystal is a JSON file — steps and selectors captured from a passing trace, not parsed out of test code. The replay runtime is a thin CDP interpreter inside this binary: no chromedp, no playwright-go, no model in the loop.

## Install

Download a binary from [Releases](https://github.com/svasenkov/greedy-guru/releases) (darwin/linux, amd64/arm64), or install with Go (vanity import `greedy.guru/greedy`):

```bash
go install greedy.guru/greedy/cmd/greedy@latest

# or build from source:
git clone https://github.com/svasenkov/greedy-guru.git && cd greedy-guru
go build ./cmd/greedy
```

## Quickstart

```bash
greedy validate crystals/login.example.json      # offline schema check, no Chrome needed
greedy crystallize --hits 3 --days 2 --pattern "login" \
    --trace trace.zip --id login --out login.json # cut IR from a green Playwright trace
greedy run --cdp http://127.0.0.1:9222 --base-url https://app-under-test/ login.json
```

`run` never starts Chrome itself — it needs an already-live DevTools endpoint: `--cdp`, `GREEDY_CDP`, a pool lease (`GREEDY_POOL`, `POST /pool/lease`), or the compose sidecar.

## Commands

| Command | What it does |
|---------|--------------|
| `greedy validate <crystal.json>` | Check a crystal against `schema/crystal.v1.json` |
| `greedy search [--path DIR] <query>` | `rg` wrapper |
| `greedy crystallize --hits N --days N --pattern TEXT [--trace trace.zip --id ID --as-id TITLE --out FILE]` | Gate a repeated task (hits/days/sessions/faker); with `--trace`, cut IR from a green Playwright trace |
| `greedy run --cdp URL --base-url URL [--parallel N] <crystal.json>` | Replay on an already-live Chrome over CDP |
| `greedy bench --base-url URL [--seq 1,5,10,25] [--parallel N] [--waves W] [--repeat R] <crystal.json>` | Held-CDP benchmark: seq queue on one client, par/waves across N clients |
| `greedy observe` / `diff` / `approve` | Crystal lifecycle in Allure TestOps (`crystal_status` field, `pending_review → live`) |

JSON on stdout; exit codes `0` ok / `1` fail / `2` usage. Full contract: `greedy help`.

## IR v1

Seven ops: `navigate`, `wait`, `fill`, `click`, `text`, `park`, `eval`. `crystallize` emits the first five from a trace; `park` (leave the tab on a URL) and `eval` (raw `Runtime.evaluate`) are hand-authored. Selectors are what resolved in the trace (`data-testid`), not `getByRole`. Schema: [`schema/crystal.v1.json`](schema/crystal.v1.json); example: [`crystals/login.example.json`](crystals/login.example.json).

```json
{ "op": "fill", "selector": "[data-testid=login-username]", "value": "user1" }
```

## Speed — measured, not a slogan

One login crystal on one hot Chrome, static fixture: **41 ms** wall ([site/bench.json](site/bench.json) is the pin SSOT, measured 2026-09-24). The full matrix (queue depth × parallelism, Mac vs the Selenoid farm) is on the [landing](https://greedy.guru) and in [`site/bench-matrix.json`](site/bench-matrix.json) — reproduced by `scripts/bench-matrix.py` + `greedy bench`.

What the numbers are *not*: not Jenkins stage time, not `POST /run` daemon time, not "Go is 10× faster than X". Held CDP, `--mode none` (no Allure on the clock), warmup discarded, quote = `median_ms`.

## What it is not

- Not a port of [greedy-token](https://github.com/svasenkov/greedy-token) (Python MCP router) — that stays Python.
- Not an MCP server. Programmatic API is this CLI's JSON stdout.
- Not a test writer — humans/Playwright write tests; greedy only freezes green runs.
- Not a page-object runtime — no living Go scenarios, no chromedp, no go-playwright.

## Development

```bash
go test ./... -short        # unit tier, no Chrome
go test ./... -p 1          # live: boots Chrome.app (macOS), google-chrome (CI), or docker pw-min
go test ./internal/cli -update   # regenerate testdata/golden/*
```

Live overrides: `CHROME_BIN`, `GREEDY_CDP`, `GREEDY_POOL`, `GREEDY_PW_MIN_IMAGE`. The TestOps lifecycle (`observe`/`approve`, `allure_id`) is used by a private test mill; the CLI itself only needs a CDP endpoint.

## Releases

Tags `v*` → GoReleaser → GitHub Release with binaries + checksums. Version stamp is injected at build time (`-X …/internal/cli.Version`); `greedy version` prints it.
