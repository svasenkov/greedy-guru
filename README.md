# greedy.guru

Жёсткий cut [greedy-token](https://github.com/svasenkov/greedy-token): IR-кристаллы и Go-раннер CDP. Не Python MCP и **не** MCP-обёртка этого CLI (осознанно, ADR 015 §7): смоук вызывается бинарником из кода/CI.

| | |
|--|--|
| Репозиторий | [svasenkov/greedy-guru](https://github.com/svasenkov/greedy-guru) |
| Домен | [greedy.guru](https://greedy.guru) (прод: Box3 static; DNS в infra-home) |
| ADR | [015](../../../docs/adr/015-greedy-guru.md) |
| План | [docs/plans/greedy-guru.md](../../../docs/plans/greedy-guru.md) |
| Фаза | `10.greedy-guru` ✓ · mill [`greedy-guru-mill`](../../../docs/plans/greedy-guru-mill.md) ✓ (не 11) · mill — колонка crystal на `/stack/` |

```bash
cd projects/greedy-token-home/greedy-guru
go test ./... -p 1         # live CDP: Mac → Chrome.app, иначе docker pw-min
go test ./... -short       # без live Chrome
# GREEDY_CDP=http://127.0.0.1:9222 go test ./... -p 1
# CHROME_BIN=/path/to/chrome go test ./... -p 1
go run ./cmd/greedy version
go run ./cmd/greedy help
go run ./cmd/greedy search --path testdata/search p1-search-marker-a1b2
go run ./cmd/greedy validate crystals/login.example.json
go run ./cmd/greedy crystallize --hits 3 --days 2 --pattern "login valid credentials"
go run ./cmd/greedy crystallize --hits 3 --days 2 --pattern "login valid credentials" \
  --trace testdata/trace/login.trace --out /tmp/login.json \
  --id login --as-id "Пользователь может войти с валидными credentials"
MILL=../../autotests-ai-multistack-home/autotests-ai-multistack-app/tests/go/tests-go-cdp/crystals
go run ./cmd/greedy validate "$MILL/login.json"
# canonical clocks (held CDP; par/waves first; cell warmup discarded; quote median_ms):
# go run ./cmd/greedy bench --cdp "$GREEDY_CDP" --base-url URL --seq 1,5,10,25 --repeat 10 --mode none "$MILL/login.json"
# GREEDY_BENCH_PARALLEL=5 GREEDY_BENCH_WAVES=2,5 GREEDY_BENCH_REPEAT=10 scripts/bench-matrix.sh
go run ./cmd/greedy observe --id login --eligible --pw-green 3
go run ./cmd/greedy diff crystals/login.example.json "$MILL/login.json"
go run ./cmd/greedy approve --proposed "$MILL/login.json" --out "$MILL/login.json" \
  --live "$MILL/login.json" --pw-green 3 --eligible
# mill (etalon clone, exec this binary — not import internal/cdp)
cd ../../autotests-ai-multistack-home/autotests-ai-multistack-app/tests/go/tests-go-cdp && go test
```

`crystallize` без `--trace` — только гейты. `--trace` — Playwright `trace.zip` / `.trace` (успешный прогон) → IR v1, не AST `.spec.ts`. Live IR в git — mill `tests/go/tests-go-cdp/crystals/` (клетка на `/stack/`). Guru `crystals/` — только `login.example.json` (не Live). JSON в mill — после ручного OK.

`observe`: 3 зелёных **PW**-launch → `pending_review` (allowlist auto-Live только `login*`). Уже `live` и fingerprint не сменился → `live`, **без PATCH**. Починка spec / новый fingerprint / падение Live → снова review. Поле TestOps `crystal_status` **не** `from_test_result`. Eligibility — tag `crystal`, не лейбл прогона. Кристалл не `@Layer("manual")`.

`approve` = diff IR + PATCH TestOps + файл JSON (не галочка в TMS). Кейс на workflow **greedy.guru crystals** (Active / Outdated), не «Автоматизированные тесты». Тонкий PATCH в tms-automator: `scripts/patch_crystal_status.py` (не bootstrap репо). Id кейсов mill-кристаллов: [`testdata/testops-cases.json`](testdata/testops-cases.json) (проект [5366](https://allure.qa.guru/project/5366/test-cases)).

`run` — к уже живому Chrome DevTools HTTP (`--cdp` или lease, флаг `--remote-allow-origins=*`). Live-тесты поднимают **PW min** (`qaguru/playwright-chromium:1.61.1-min`): Chromium из образа + CDP, не Playwright WS `:3000`, не WebDriver chrome-min. WD-образ — если будем кристаллизовать Selenium. Фикстура — `host.docker.internal`. Override: `CHROME_BIN`, `GREEDY_PW_MIN_IMAGE`, `GREEDY_CDP` (явный URL, не хардкод в коде). Без `--cdp`: `GREEDY_CDP` (только N=1), иначе N× `POST /pool/lease {protocol:cdp}` (`GREEDY_POOL`), иначе sidecar compose (N=1). `--parallel N` = N CDP, не вкладки. `--mode none`: Allure generate не на стене. Цифра на лендинге — **1×** `site/bench.json` (`testdata/app-live`, ~90 ms). Box1 N=5 на SPA — другая шкала (~300 ms стена), не в `bench.json`.

`bench` — один протокол для листа. Сессии держатся до конца команды. Warmup команды и **первый прогон каждой клетки** выбрасываются. par/waves **до** очереди (простой Chrome не остывает). JSON: `best_ms` (min), `median_ms`, `runs_ms`. **Цитата и пропорции — медиана**, не min разных клеток. `--repeat` по умолчанию 3; лист Mac vs [selenoid.qa.guru](https://selenoid.qa.guru) — `--repeat 10`. Часы: `Run` / `RunParallel` на уже открытом CDP — **без Dial, без park демона, без клетки Jenkins**. `--seq 1,5,10,25` — очередь на одном Client. `--parallel 5` — одна волна на 5 Client (стена = max). `--waves 2,5` — 2 или 5 таких волн **на тех же** Client, без нового lease. Не смешивать с `POST /run` (там park между прогонами). SPA `/stack/` и `testdata/app-live` — разные строки. Лендинг остаётся **1×** `site/bench.json` (~90 ms), не эта матрица.

```bash
# лист (SPA), 5 CDP из пула; на Box1 сначала отпустить hot-cdp-daemon
# GREEDY_RESET=on (default): без park между Run форма логина пропадает. off — только daemon /run.
# IR: mill tests-go-cdp/crystals/login.json (not guru/crystals)
GREEDY_POOL=http://selenoid-pool:9090 GREEDY_CDP_FALLBACK=off \
  greedy bench --base-url https://autotests.ai/stack/backend-java-spring/frontend-typescript-react/ \
  --seq 1,5,10,25 --parallel 5 --waves 2,5 --repeat 10 --mode none \
  tests/go/tests-go-cdp/crystals/login.json
```

Hot CDP — **5 слотов** selenoid-pool (`pool-hot-cdp-min-1`…`5`), live на Box1 / [selenoid.qa.guru](https://selenoid.qa.guru) (hot **8/8**). Не отдельный пул. Пулы: cold / warm / hot. Тот же `-min` Chromium, DevTools, не `:3000`. SSOT: `projects/selenoid-home/selenoid-pool/`. Sidecar [`docker-compose.hot-cdp.yml`](docker-compose.hot-cdp.yml) — fallback **1×** на 16443 (не вместе с `hot-cdp-min-1`). Не стенд `ensure.py`. DevTools URL не в Материалы чата.

Не chromedp, не Node. Ячейка TS Playwright — другой чат. Mill: etalon `tests/go/tests-go-cdp` (`role: mill`, `layers: [crystal]`, `in_stack: true`) — **SSOT live** `crystals/*.json` + `exec greedy run`, не `import internal/cdp`.

Хабы: nested clone `../greedy-token/` остаётся прототипом Cursor. Этот каталог — продукт домена.
