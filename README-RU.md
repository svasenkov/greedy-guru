# greedy

**Один раз сохраните зелёный прогон Playwright — дальше его проигрывает один Go-бинарник по CDP, без Node, npm и Playwright на CI-агенте.**

[EN → README.md](README.md) · Сайт: [greedy.guru](https://greedy.guru)

## Быстрый старт

Бинарник — из [Releases](https://github.com/svasenkov/greedy-guru/releases) (macOS/Linux, amd64/arm64) или через Go (vanity-импорт `greedy.guru/greedy`):

```bash
go install greedy.guru/greedy/cmd/greedy@latest
```

```bash
greedy validate crystals/login.example.json          # проверка по схеме, Chrome не нужен
greedy crystallize --hits 3 --days 2 --pattern "login valid credentials" \
    --trace trace.zip --id login --out login.json     # зелёная трасса Playwright → кристалл
greedy run --cdp http://127.0.0.1:9222 --base-url https://app-under-test/ login.json
```

`trace.zip` даёт зелёный прогон Playwright с `--trace on`. `run` не поднимает Chrome сам: укажите уже запущенный браузер с DevTools-эндпоинтом — `--cdp URL`, `GREEDY_CDP`, пул браузеров (`GREEDY_POOL`, `POST /pool/lease`) или Chrome-контейнер из [`docker-compose.hot-cdp.yml`](docker-compose.hot-cdp.yml).

Сборка из исходников: `git clone https://github.com/svasenkov/greedy-guru.git && cd greedy-guru && go build ./cmd/greedy`.

## Как это работает

```
один раз:        тест на Playwright --trace on → trace.zip → greedy crystallize → login.json (кристалл, в git)
каждый прогон:   greedy run login.json → CDP → горячий Chrome → JSON-результат на stdout
```

- **Кристалл** — JSON-файл с шагами и селекторами, которые реально отработали в зелёной трассе, а не разобраны из кода теста.
- **Горячий Chrome** — уже запущенный браузер; greedy подключается к нему по [CDP](https://chromedevtools.github.io/devtools-protocol/) (Chrome DevTools Protocol) и сам браузер не запускает.
- **Проигрыватель** — тонкий CDP-интерпретатор внутри бинарника: без chromedp, без playwright-go, без ИИ-модели в контуре.
- **Ревью** — кристалл уходит в Live только после ревью в Allure TestOps (`observe` / `diff` / `approve`); поменяли тест — он снова на ревью.

## Команды

| Команда | Что делает |
|---------|------------|
| `greedy validate <crystal.json>` | Проверка кристалла по [`schema/crystal.v1.json`](schema/crystal.v1.json) |
| `greedy crystallize --hits N --days N [--sessions N] --pattern TEXT` | **Гейт:** достаточно ли сценарий стабилен, чтобы его замораживать? Нужны ≥3 зелёных прогона за ≥2 дня или ≥2 сессии, никаких faker-данных, паттерн из ≥3 слов. Печатает `eligible`, файлов не пишет |
| `greedy crystallize … --trace trace.zip --id ID [--as-id TITLE] [--out FILE]` | **Нарезка из трассы:** тот же гейт, затем кристалл из зелёной трассы Playwright |
| `greedy run --cdp URL --base-url URL [--parallel N] <crystal.json>` | Проигрывание на горячем Chrome по CDP; `--parallel N` — N браузеров, по кристаллу на каждый |
| `greedy bench --base-url URL [--seq 1,5,10,25] [--parallel N] [--waves W] [--repeat R] <crystal.json>` | Бенчмарк: очередь на одном Chrome (`--seq`) против параллели на N (`--parallel`, `--waves`) |
| `greedy observe` / `diff` / `approve` | Ревью в Allure TestOps: 3 зелёных прогона → `pending_review`; `diff` показывает, что изменилось в шагах; `approve` ставит `live` и записывает JSON |
| `greedy search [--path DIR] [--glob GLOB] <query>` | Вспомогательная, не часть записи и проигрывания: `rg` (ripgrep) с JSON-выводом — `path`, `line`, `text`. Нужен `rg` в `PATH` |

На stdout — JSON; коды выхода `0` ok / `1` fail / `2` usage. Полный контракт: `greedy help`.

## Формат кристалла (IR v1)

Семь операций: `navigate`, `wait`, `fill`, `click`, `text`, `park`, `eval`. Из трассы `crystallize` порождает первые пять; `park` (оставить вкладку на URL) и `eval` (выполнить JavaScript через `Runtime.evaluate`) пишутся руками. Селекторы — те, что реально отработали в трассе (`data-testid`), не `getByRole`. Схема: [`schema/crystal.v1.json`](schema/crystal.v1.json); полный пример: [`crystals/login.example.json`](crystals/login.example.json).

```json
{ "op": "fill", "selector": "[data-testid=login-username]", "value": "user1" }
```

## Скорость — замер, не слоган

**39 ms** — кристалл логина из 7 шагов на одном горячем Chrome, статичная тестовая страница (`testdata/app-live`). Источник: [`site/bench.json`](site/bench.json) (`wall_ms`, замер 2026-09-24); лендинг берёт цифры из этого файла.

На живом SPA тот же логин идёт дольше. В [`site/bench-matrix.json`](site/bench-matrix.json) — пачки из 1–25 логинов: очередь на одном Chrome против параллели на 5, на Mac и на ферме Selenoid; воспроизводится `scripts/bench-matrix.py` + `greedy bench`, таблица — на [лендинге](https://greedy.guru/#bench).

Как мерили: соединение по CDP открыто до старта таймера, `--mode none` держит Allure вне замера, cookies и storage сбрасываются перед каждым прогоном, прогрев и первый прогон каждой ячейки выброшены; в таблице — `median_ms` и рядом `best_ms`. Чем эти цифры не являются: временем Jenkins-стейджа, временем `POST /run` демона, «Go в 10 раз быстрее X».

## Чем это не является

- Не порт [greedy-token](https://github.com/svasenkov/greedy-token) (Python MCP-роутер) — тот остаётся на Python.
- Не MCP-сервер. Программный API — JSON на stdout этого CLI.
- Не пишет тесты — тесты пишут люди на Playwright; greedy только замораживает зелёные прогоны.
- Не рантайм page-object — ни рукописных Go-сценариев, ни chromedp, ни playwright-go.

## Разработка

```bash
go test ./... -short        # юнит-ярус, без Chrome
go test ./... -p 1          # live: поднимает Chrome.app (macOS), google-chrome (CI) или docker pw-min
go test ./internal/cli -update   # перегенерировать testdata/golden/*
```

Переопределения live-окружения: `CHROME_BIN`, `GREEDY_CDP`, `GREEDY_POOL`, `GREEDY_PW_MIN_IMAGE`. Жизненный цикл в TestOps (`observe`/`approve`, `allure_id`) использует приватный CI-контур; самому CLI нужен только CDP-эндпоинт.

## Релизы

Теги `v*` → GoReleaser → GitHub Release с бинарниками и checksums. Версия вшивается при сборке (`-X …/internal/cli.Version`); печатает `greedy version`.
