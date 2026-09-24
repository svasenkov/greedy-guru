# greedy

**Тесты пишутся на Playwright. Прогоняются как кристаллы на горячем Chrome — один Go-бинарник, чистый CDP, без Node.**

[EN → README.md](README.md) · Сайт: [greedy.guru](https://greedy.guru)

```
Тест на Playwright → trace.zip зелёного прогона → greedy crystallize → login.json (IR)
                                                                            ↓
Ревью в TestOps                                        greedy run → горячий Chrome (CDP)
```

Кристалл — это JSON: шаги и селекторы, снятые с трассы успешного прогона, а не разобранные из кода теста. Раннер — тонкий CDP-интерпретатор внутри этого бинарника: без chromedp, без playwright-go, без модели в контуре.

## Установка

Бинарник из [Releases](https://github.com/svasenkov/greedy-guru/releases) (darwin/linux, amd64/arm64) или установка через Go (vanity-импорт `greedy.guru/greedy`):

```bash
go install greedy.guru/greedy/cmd/greedy@latest

# или сборка из исходников:
git clone https://github.com/svasenkov/greedy-guru.git && cd greedy-guru
go build ./cmd/greedy
```

## Быстрый старт

```bash
greedy validate crystals/login.example.json      # проверка по схеме, Chrome не нужен
greedy crystallize --hits 3 --days 2 --pattern "login" \
    --trace trace.zip --id login --out login.json # нарезка IR из зелёной трассы Playwright
greedy run --cdp http://127.0.0.1:9222 --base-url https://app-under-test/ login.json
```

`run` не поднимает Chrome сам — нужен уже живой DevTools-эндпоинт: `--cdp`, `GREEDY_CDP`, лиз из пула (`GREEDY_POOL`, `POST /pool/lease`) или сайдкар из compose.

## Команды

| Команда | Что делает |
|---------|------------|
| `greedy validate <crystal.json>` | Проверка кристалла по `schema/crystal.v1.json` |
| `greedy search [--path DIR] <query>` | Обёртка над `rg` |
| `greedy crystallize --hits N --days N --pattern TEXT [--trace trace.zip --id ID --as-id TITLE --out FILE]` | Гейт повторяющейся задачи (hits/days/sessions/faker); с `--trace` — нарезка IR из зелёной трассы Playwright |
| `greedy run --cdp URL --base-url URL [--parallel N] <crystal.json>` | Прогон на уже запущенном Chrome по CDP |
| `greedy bench --base-url URL [--seq 1,5,10,25] [--parallel N] [--waves W] [--repeat R] <crystal.json>` | Бенч на удерживаемых CDP: очередь на одном клиенте, par/waves на N клиентах |
| `greedy observe` / `diff` / `approve` | Жизненный цикл кристалла в Allure TestOps (поле `crystal_status`, `pending_review → live`) |

На stdout — JSON; коды выхода `0` ok / `1` fail / `2` usage. Полный контракт: `greedy help`.

## IR v1

Семь операций: `navigate`, `wait`, `fill`, `click`, `text`, `park`, `eval`. Из трассы `crystallize` порождает первые пять; `park` (оставить вкладку на URL) и `eval` (голый `Runtime.evaluate`) пишутся руками. Селекторы — те, что реально отработали в трассе (`data-testid`), не `getByRole`. Схема: [`schema/crystal.v1.json`](schema/crystal.v1.json); пример: [`crystals/login.example.json`](crystals/login.example.json).

```json
{ "op": "fill", "selector": "[data-testid=login-username]", "value": "user1" }
```

## Скорость — замер, не слоган

Один кристалл логина на одном горячем Chrome, статичная страница: **41 ms** wall ([site/bench.json](site/bench.json) — SSOT пина, замер 2026-09-24). Полная матрица (глубина очереди × параллельность, Mac против фермы Selenoid) — на [лендинге](https://greedy.guru) и в [`site/bench-matrix.json`](site/bench-matrix.json); воспроизводится `scripts/bench-matrix.py` + `greedy bench`.

Чем эти цифры не являются: не временем Jenkins-стейджа, не временем `POST /run` демона, не «Go в 10 раз быстрее X». Удерживаемый CDP, `--mode none` (Allure вне замера), warmup выброшен, в цитату идёт `median_ms`.

## Чем это не является

- Не порт [greedy-token](https://github.com/svasenkov/greedy-token) (Python MCP-роутер) — тот остаётся на Python.
- Не MCP-сервер. Программный API — JSON на stdout этого CLI.
- Не писатель тестов — тесты пишут люди/Playwright; greedy только замораживает зелёные прогоны.
- Не рантайм page-object — живых Go-сценариев нет, ни chromedp, ни go-playwright.

## Разработка

```bash
go test ./... -short        # юнит-ярус, без Chrome
go test ./... -p 1          # live: поднимает Chrome.app (macOS), google-chrome (CI) или docker pw-min
go test ./internal/cli -update   # перегенерировать testdata/golden/*
```

Переопределения live-окружения: `CHROME_BIN`, `GREEDY_CDP`, `GREEDY_POOL`, `GREEDY_PW_MIN_IMAGE`. Жизненный цикл в TestOps (`observe`/`approve`, `allure_id`) используется приватной мельницей тестов; самому CLI нужен только CDP-эндпоинт.

## Релизы

Теги `v*` → GoReleaser → GitHub Release с бинарниками и checksums. Версия вшивается при сборке (`-X …/internal/cli.Version`); печатает `greedy version`.
