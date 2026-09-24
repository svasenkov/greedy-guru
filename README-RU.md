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

Бинарник из [Releases](https://github.com/svasenkov/greedy-guru/releases) (darwin/linux, amd64/arm64) или сборка из исходников:

```bash
git clone https://github.com/svasenkov/greedy-guru.git && cd greedy-guru
go build ./cmd/greedy
# или: go install github.com/svasenkov/greedy-guru/cmd/greedy@latest
```

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

Шесть операций: `navigate`, `wait`, `fill`, `click`, `text`, `park`. Селекторы — те, что реально отработали в трассе (`data-testid`), не `getByRole`. Схема: [`schema/crystal.v1.json`](schema/crystal.v1.json); пример: [`crystals/login.example.json`](crystals/login.example.json).

```json
{ "op": "fill", "selector": "[data-testid=login-input]", "value": "user1" }
```

## Скорость — замер, не слоган

Один кристалл логина на одном горячем Chrome, статичная страница: **~90 ms** wall ([site/bench.json](site/bench.json)). Полная матрица (глубина очереди × параллельность, Mac против фермы Selenoid) — на [лендинге](https://greedy.guru) и в [`site/bench-matrix.json`](site/bench-matrix.json); воспроизводится `scripts/bench-matrix.py` + `greedy bench --repeat 10`.

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
