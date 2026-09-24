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

Download a binary from [Releases](https://github.com/svasenkov/greedy-guru/releases) (darwin/linux, amd64/arm64), or build from source:

```bash
git clone https://github.com/svasenkov/greedy-guru.git && cd greedy-guru
go build ./cmd/greedy
# or: go install github.com/svasenkov/greedy-guru/cmd/greedy@latest
```

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

Six ops: `navigate`, `wait`, `fill`, `click`, `text`, `park`. Selectors are what resolved in the trace (`data-testid`), not `getByRole`. Schema: [`schema/crystal.v1.json`](schema/crystal.v1.json); example: [`crystals/login.example.json`](crystals/login.example.json).

```json
{ "op": "fill", "selector": "[data-testid=login-input]", "value": "user1" }
```

## Speed — measured, not a slogan

One login crystal on one hot Chrome, static fixture: **~90 ms** wall ([site/bench.json](site/bench.json)). The full matrix (queue depth × parallelism, Mac vs the Selenoid farm) is on the [landing](https://greedy.guru) and in [`site/bench-matrix.json`](site/bench-matrix.json) — reproduced by `scripts/bench-matrix.py` + `greedy bench --repeat 10`.

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
