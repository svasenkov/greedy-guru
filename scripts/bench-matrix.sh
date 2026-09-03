#!/usr/bin/env bash
# Canonical greedy.guru clocks: same as `greedy bench` help (held CDP, par/waves first).
# Does not start Chrome, docker, or Jenkins. On Box1: stop hot-cdp-daemon first, then restore.
# Sheet quote: GREEDY_BENCH_REPEAT=10 GREEDY_BENCH_PARALLEL=5 GREEDY_BENCH_WAVES=2,5
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
SPA="${GREEDY_BASE_URL:-https://autotests.ai/stack/backend-java-spring/frontend-typescript-react/}"
MILL_LOGIN="$ROOT/../../autotests-ai-multistack-home/autotests-ai-multistack-app/tests/go/tests-go-cdp/crystals/login.json"
if [[ -n "${GREEDY_CRYSTALS:-}" ]]; then
  MILL_LOGIN="$GREEDY_CRYSTALS/login.json"
fi
CRYSTAL="${1:-$MILL_LOGIN}"
REPEAT="${GREEDY_BENCH_REPEAT:-3}"
PARALLEL="${GREEDY_BENCH_PARALLEL:-0}"
WAVES="${GREEDY_BENCH_WAVES:-}"
cd "$ROOT"
args=(bench --base-url "$SPA" --seq "${GREEDY_BENCH_SEQ:-1,5,10,25}" --repeat "$REPEAT" --mode none)
if [[ "$PARALLEL" != "0" ]]; then
  args+=(--parallel "$PARALLEL")
fi
if [[ -n "$WAVES" ]]; then
  args+=(--waves "$WAVES")
fi
exec go run ./cmd/greedy "${args[@]}" "$CRYSTAL"
