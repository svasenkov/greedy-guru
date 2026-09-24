package cli

// Version is the CLI version stamp. Release builds override it via
// -ldflags "-X greedy.guru/greedy/internal/cli.Version=vX.Y.Z" (GoReleaser).
var Version = "0.6.0"

const ExitOK = 0
const ExitFail = 1
const ExitUsage = 2

const Usage = `greedy — greedy.guru CLI (hard cut)

  greedy version
  greedy help
  greedy search [--path DIR] [--glob GLOB] <query>
  greedy validate <crystal.json>
  greedy crystallize --hits N [--days N] [--sessions N] [--faker] --pattern TEXT [--trace FILE --id ID [--as-id TITLE] [--out FILE]]
  greedy run [--cdp URL ...] --base-url URL [--parallel N] [--mode none] <crystal.json>
  greedy bench [--cdp URL ...] --base-url URL [--seq 1,5,10,25] [--parallel N] [--waves 2,5] [--repeat 3] [--mode none] <crystal.json>
  greedy observe --id ID --pw-green N [--eligible] [--pw-fail] [--spec-fix] [--live-fail] [--fingerprint HEX] [--live-fingerprint HEX] [--was STATUS] [--layer e2e] [--testcase ID]
  greedy diff <live.json> <proposed.json>
  greedy approve --proposed FILE --out FILE [--live FILE] --pw-green N --eligible [--was pending_review] [--layer e2e] [--testcase ID] [--endpoint URL]

Exit: 0 ok · 1 fail · 2 usage. search/validate/crystallize/run/bench/observe/diff/approve print JSON on stdout.
search wraps rg (not a built-in grep). crystallize checks gates; --trace FILE maps Playwright trace.zip → IR v1 (not .spec.ts AST). --id is the file slug (no default id). --as-id is the living PW test() title; if omitted, taken from the trace.
run talks CDP to already-live Chrome (no Node, no chromedp). --cdp omitted: GREEDY_CDP (N=1), else N× POST /pool/lease {protocol:cdp} (GREEDY_POOL, default host orchestrator), else sidecar compose (N=1 only). --parallel N: N CDP endpoints (N --cdp, or N pool leases). Not N tabs on one Chrome, not N host Chrome.app from this binary. --mode none: Allure generate is not on the wall. This binary does not start docker slots or ensure.py stands.
bench: held CDP. Global warmup + first of each cell discarded. par/waves before seq (idle Chrome stay hot). --repeat counted runs; best_ms=min, median_ms, runs_ms. seq = N Run on one Client. --parallel N = one RunParallel (max worker). --waves W = W RunParallel on those same Clients (no re-lease). No daemon park. Not Jenkins Stage View. Not testdata/app-live 90 ms unless that is --base-url.
observe: 3 PW green → pending_review (login allowlist may auto-Live). Already live and fingerprint unchanged → live, no PATCH. spec fix / new fingerprint / Live fail → review. Field crystal_status is not from_test_result. Crystal is not @Layer("manual").
approve = IR diff + TestOps PATCH + JSON file (not a TMS tick alone). CDP ≠ ensure.py.
`
