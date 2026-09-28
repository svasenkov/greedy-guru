# greedy

**Record a green Playwright run once, then replay it with a single Go binary over CDP — no Node, npm or Playwright on the CI agent.**

[RU version → README-RU.md](README-RU.md) · Site: [greedy.guru](https://greedy.guru)

## Quickstart

Grab a binary from [Releases](https://github.com/svasenkov/greedy-guru/releases) (macOS/Linux, amd64/arm64) or install with Go (vanity import `greedy.guru/greedy`):

```bash
go install greedy.guru/greedy/cmd/greedy@latest
```

```bash
greedy validate crystals/login.example.json          # schema check, no Chrome needed
greedy crystallize --hits 3 --days 2 --pattern "login valid credentials" \
    --trace trace.zip --id login --out login.json     # green Playwright trace → crystal
greedy run --cdp http://127.0.0.1:9222 --base-url https://app-under-test/ login.json
```

`trace.zip` comes from a green Playwright run with `--trace on`. `run` never starts Chrome itself: point it at a browser that is already running with a DevTools endpoint — `--cdp URL`, `GREEDY_CDP`, a browser pool (`GREEDY_POOL`, `POST /pool/lease`), or the Chrome container from [`docker-compose.hot-cdp.yml`](docker-compose.hot-cdp.yml).

Build from source: `git clone https://github.com/svasenkov/greedy-guru.git && cd greedy-guru && go build ./cmd/greedy`.

## How it works

```
once:       Playwright test --trace on → trace.zip → greedy crystallize → login.json (crystal, in git)
every run:  greedy run login.json → CDP → hot Chrome → JSON result on stdout
```

- **Crystal** — a JSON file with the steps and selectors that actually resolved in a green trace, not parsed from test code.
- **Hot Chrome** — a browser that is already running; greedy connects to it over [CDP](https://chromedevtools.github.io/devtools-protocol/) (Chrome DevTools Protocol) and never launches one.
- **Replay** — a thin CDP interpreter inside this binary: no chromedp, no playwright-go, no AI model in the loop.
- **Review** — a crystal goes live only after review in Allure TestOps (`observe` / `diff` / `approve`); change the test and it goes back to review.

## Commands

| Command | What it does |
|---------|--------------|
| `greedy validate <crystal.json>` | Check a crystal against [`schema/crystal.v1.json`](schema/crystal.v1.json) |
| `greedy crystallize --hits N --days N [--sessions N] --pattern TEXT` | **Gate:** is the scenario stable enough to freeze? Needs ≥3 green runs over ≥2 days or ≥2 sessions, no faker data, a pattern of ≥3 words. Prints `eligible`, writes nothing |
| `greedy crystallize … --trace trace.zip --id ID [--as-id TITLE] [--out FILE]` | **Trace-cut:** the same gate, then cut the crystal from a green Playwright trace |
| `greedy run --cdp URL --base-url URL [--parallel N] <crystal.json>` | Replay on hot Chrome over CDP; `--parallel N` = N browsers, one crystal each |
| `greedy bench --base-url URL [--seq 1,5,10,25] [--parallel N] [--waves W] [--repeat R] <crystal.json>` | Benchmark: a queue on one Chrome (`--seq`) vs parallel runs on N (`--parallel`, `--waves`) |
| `greedy observe` / `diff` / `approve` | Review in Allure TestOps: 3 green runs → `pending_review`; `diff` shows what changed in the steps; `approve` sets `live` and writes the JSON |
| `greedy search [--path DIR] [--glob GLOB] <query>` | Side helper, not part of record/replay: `rg` (ripgrep) with JSON output — `path`, `line`, `text`. Needs `rg` in `PATH` |

JSON on stdout; exit codes `0` ok / `1` fail / `2` usage. Full contract: `greedy help`.

## Crystal format (IR v1)

Seven ops: `navigate`, `wait`, `fill`, `click`, `text`, `park`, `eval`. `crystallize` emits the first five from a trace; `park` (leave the tab on a URL) and `eval` (run JavaScript via `Runtime.evaluate`) are hand-written. Selectors are what resolved in the trace (`data-testid`), not `getByRole`. Schema: [`schema/crystal.v1.json`](schema/crystal.v1.json); full example: [`crystals/login.example.json`](crystals/login.example.json).

```json
{ "op": "fill", "selector": "[data-testid=login-username]", "value": "user1" }
```

## Speed — measured, not a slogan

**39 ms** — a 7-step login crystal on one hot Chrome against a static fixture page (`testdata/app-live`). Source: [`site/bench.json`](site/bench.json) (`wall_ms`, measured 2026-09-24); the landing page reads its numbers from that file.

On a live SPA the same login takes longer. [`site/bench-matrix.json`](site/bench-matrix.json) holds batches of 1–25 logins as a queue on one Chrome vs parallel on 5, on a Mac and on the Selenoid farm — reproduced with `scripts/bench-matrix.py` + `greedy bench`, shown as a table on the [landing](https://greedy.guru/#bench).

How it is measured: the CDP connection is opened before the clock starts, `--mode none` keeps Allure off the clock, cookies and storage are reset before each run, warmup and the first run of every cell are discarded; the table quotes `median_ms` with `best_ms` next to it. What the numbers are *not*: Jenkins stage time, `POST /run` daemon time, or "Go is 10× faster than X".

## What it is not

- Not a port of [greedy-token](https://github.com/svasenkov/greedy-token) (Python MCP router) — that stays Python.
- Not an MCP server. The programmatic API is this CLI's JSON stdout.
- Not a test writer — people write tests in Playwright; greedy only freezes green runs.
- Not a page-object runtime — no hand-written Go scenarios, no chromedp, no playwright-go.

## Development

```bash
go test ./... -short        # unit tier, no Chrome
go test ./... -p 1          # live: boots Chrome.app (macOS), google-chrome (CI), or docker pw-min
go test ./internal/cli -update   # regenerate testdata/golden/*
```

Live overrides: `CHROME_BIN`, `GREEDY_CDP`, `GREEDY_POOL`, `GREEDY_PW_MIN_IMAGE`. The TestOps lifecycle (`observe`/`approve`, `allure_id`) is used by a private CI pipeline; the CLI itself only needs a CDP endpoint.

## Releases

Tags `v*` → GoReleaser → GitHub Release with binaries + checksums. Version stamp is injected at build time (`-X …/internal/cli.Version`); `greedy version` prints it.
