# FFUUZZ: обзорная прогулка по кодовой базе

> Этот документ -- учебник-путеводитель по исходному коду проекта **FFUUZZ**.
> Он написан для разработчика уровня джуна и построен по принципу
> «от простого к сложному». Вместо сухого перечисления пакетов мы пройдём
> по пользовательским сценариям и увидим, **какой код за них отвечает**.

---

## Оглавление

1. [Что такое FFUUZZ одним абзацем](#1-что-такое-ffuuzz-одним-абзацем)
2. [Технологический стек](#2-технологический-стек)
3. [Карта каталогов](#3-карта-каталогов)
4. [Доменная модель: ключевые типы данных](#4-доменная-модель-ключевые-типы-данных)
5. [Точка входа и CLI-команды](#5-точка-входа-и-cli-команды)
6. [Сценарий 1: Запись трафика через MITM-прокси](#6-сценарий-1-запись-трафика-через-mitm-прокси)
7. [Сценарий 2: Создание и запуск кампании фаззинга](#7-сценарий-2-создание-и-запуск-кампании-фаззинга)
8. [Сценарий 3: Мутация запросов](#8-сценарий-3-мутация-запросов)
9. [Сценарий 4: Повтор (replay) и обнаружение аномалий](#9-сценарий-4-повтор-replay-и-обнаружение-аномалий)
10. [Сценарий 5: Триаж -- подтверждение и минимизация находок](#10-сценарий-5-триаж----подтверждение-и-минимизация-находок)
11. [Сценарий 6: Воспроизведение находки по запросу (Reproduce)](#11-сценарий-6-воспроизведение-находки-по-запросу-reproduce)
12. [Сценарий 7: Работа с веб-интерфейсом и SSE-стримингом](#12-сценарий-7-работа-с-веб-интерфейсом-и-sse-стримингом)
13. [Как данные хранятся: слой базы данных](#13-как-данные-хранятся-слой-базы-данных)
14. [Нормализация эндпоинтов и статистическое схлопывание](#14-нормализация-эндпоинтов-и-статистическое-схлопывание)
15. [Метрики и мониторинг](#15-метрики-и-мониторинг)
16. [Конфигурация приложения](#16-конфигурация-приложения)
17. [Жизненный цикл: запуск и грациозная остановка](#17-жизненный-цикл-запуск-и-грациозная-остановка)
18. [Фронтенд (web/)](#18-фронтенд-web)
19. [Шпаргалка: где что искать](#19-шпаргалка-где-что-искать)

---

## 1. Что такое FFUUZZ одним абзацем

**FFUUZZ** -- это инструмент для тестирования безопасности веб-приложений.
Он работает по принципу *"запиши и мутируй"*:

1. Пользователь направляет свой HTTP/HTTPS-трафик через встроенный
   MITM-прокси. Прокси записывает каждый запрос и ответ.
2. Из записанного трафика формируются *сид-записи (seeds)* -- они становятся
   шаблонами для фаззинга.
3. Движок (engine) берёт эти шаблоны, *мутирует* их (меняет URL-пути,
   заголовки, JSON-тела и т.д.) и отправляет мутированные запросы на
   тестируемое приложение.
4. Ответы анализируются *детекторами аномалий*: таймауты, 5xx-ошибки,
   деградация латентности, совпадения по regex.
5. Обнаруженные аномалии проходят *триаж* (подтверждение, минимизация)
   и сохраняются как *находки (findings)* с артефактами для воспроизведения.

---

## 2. Технологический стек

| Слой | Технология |
|---|---|
| Язык бэкенда | Go 1.25 |
| HTTP-фреймворк (API) | [Gin](https://github.com/gin-gonic/gin) |
| База данных | PostgreSQL 16, `jmoiron/sqlx`, `golang-migrate` |
| Метрики | Prometheus (`prometheus/client_golang`) |
| Логирование | `rs/zerolog` (структурированный JSON-лог) |
| Кэш сертификатов | `hashicorp/golang-lru/v2` + `x/sync/singleflight` |
| Фронтенд | React + Vite + TailwindCSS + DaisyUI + TanStack Query |
| Встраивание фронтенда | Go `embed` (SPA отдаётся бинарником) |
| Контейнеризация | Docker Compose (пока только PostgreSQL) |

---

## 3. Карта каталогов

```
ffuuzz/
├── cmd/ffuuzz/main.go          # Точка входа: создаёт CLI и вызывает Run()
├── internal/
│   ├── model/model.go          # Все доменные типы (Recording, Campaign, Finding...)
│   ├── config/config.go        # Загрузка конфигурации (env + flags)
│   ├── cli/                    # Команды CLI: serve, proxy, record
│   ├── mitm/mitm.go            # MITM HTTP/HTTPS-прокси
│   ├── recorder/recorder.go    # Запись перехваченного трафика (JSONL / DB)
│   ├── endpoint/               # Нормализация путей и статистическое схлопывание
│   ├── corpus/corpus.go        # Загрузка сидов и расчёт baseline-метрик
│   ├── engine/                 # Оркестрация кампании: workers, rate-limit, reproduce
│   ├── mutate/                 # Мутаторы: URI, headers, JSON body, params, sequence
│   ├── replayer/               # Отправка мутированных запросов на цель
│   ├── anomaly/detector.go     # Детекторы аномалий (timeout, 5xx, latency, regex)
│   ├── triage/triage.go        # Подтверждение и минимизация находок
│   ├── api/                    # REST API (Gin): CRUD + SSE + SPA
│   ├── db/                     # Слой хранения: PostgreSQL + миграции
│   ├── store/store.go          # Управление TLS-сертификатами (Root CA + leaf)
│   ├── diff/diff.go            # Сравнение двух HTTP-транзакций
│   ├── report/report.go        # Агрегация статистики из JSONL-лога
│   ├── metrics/metrics.go      # Prometheus-метрики
│   ├── httputil/               # Утилиты HTTP (server builder, request ID, hop-by-hop)
│   └── logging/logging.go      # Инициализация zerolog
├── web/                        # React SPA (Vite + TailwindCSS)
│   ├── embed.go                # Go-файл для embed.FS
│   └── dist/                   # Собранный фронтенд (вкомпилируется в бинарник)
├── certs/                      # Root CA и leaf-сертификаты (создаются при запуске)
├── artifacts/                  # Артефакты (JSON-файлы с данными для воспроизведения)
├── docs/                       # Документация
├── Makefile                    # Сборка, линтинг, тесты
├── docker-compose.yml          # PostgreSQL в контейнере
├── go.mod / go.sum             # Go-модуль
└── .github/workflows/          # CI (тесты на GitHub Actions)
```

---

## 4. Доменная модель: ключевые типы данных

Все основные типы данных живут в одном файле:
`internal/model/model.go`.
Это «словарь» всего проекта -- если ты видишь незнакомый тип, загляни сюда.

### RecordingSession и Exchange

```
RecordingSession
├── ID, CreatedAt, SchemaVersion
├── Target: TargetInfo {Scheme, Host, Port, Path}
└── Entries: []Exchange
        ├── RequestID, StartedAt, DurationMs
        ├── Request:  RequestData  {Method, Path, Query, Headers, BodyB64}
        └── Response: ResponseData {Status, Headers, BodyB64}
```

**RecordingSession** -- это одна «сессия записи». Она привязана к конкретному
*эндпоинту* (scheme + host + port + нормализованный path).
Внутри сессии лежат **Exchange** -- пары запрос/ответ, записанные прокси.

### Campaign

```
Campaign
├── ID, Name, Status (CREATED → STARTING → RUNNING → FINISHED/STOPPED/FAILED)
├── RecordingIDs: []string          -- какие записи используются как сиды
├── Config: CampaignConfig
│       ├── Target.BaseURL
│       ├── Limits {Workers, RPS, MaxTests, DurationSec, ReqTimeoutMs}
│       ├── Mutations {PathQuery, Headers, JSONBody, Params, Sequence, Intensity}
│       ├── Anomaly {Detect5xx, LatencyMultiplier, RegexPatterns}
│       ├── Triage {ConfirmRuns, EnableMinimization}
│       └── ExtractionRules       -- для stateful replay ({{переменные}})
└── Progress {TestsDone, FindingsTotal}
```

**Campaign** -- центральная сущность фаззинга. Она связывает записи,
конфигурацию и результаты (находки) в единое целое.

### Finding и Artifact

```
Finding
├── ID, CampaignID, Type (TIMEOUT | SERVER_ERROR | LATENCY_REGRESSION | REGEX_MATCH)
├── Status (UNCONFIRMED | CONFIRMED)
├── Signature               -- ключ дедупликации
├── Method, Endpoint
├── Details {BaselineMs, ObservedMs, TimeoutMs, HTTPStatus}
├── MutationType, MutationPayload
└── ReproduceStatus (PENDING → ENQUEUED → RUNNING → CONFIRMED / NOT_REPRODUCED / FAILED)

Artifact
├── ID, FindingID
├── FilePath                -- относительный путь к JSON-файлу в artifacts/
└── SizeBytes
```

**Finding** -- это обнаруженная аномалия. **Artifact** -- файл на диске
с полным набором данных для её воспроизведения (ArtifactPayload).

---

## 5. Точка входа и CLI-команды

**Файл**: `cmd/ffuuzz/main.go`

```go
func main() {
    c := cli.New(os.Stdout, os.Stderr)
    os.Exit(c.Run(os.Args[1:]))
}
```

Всё просто: создаётся объект CLI и вызывается `Run()` с аргументами.
Код `Run()` в `internal/cli/cli.go` разбирает первый аргумент (команду):

| Команда | Файл | Что делает |
|---|---|---|
| `serve` | `cli/serve.go` | Полный запуск: прокси + API + engine + web UI |
| `proxy` | `cli/proxy.go` | Только MITM-прокси (dev-режим), пишет в JSONL-файл |
| `record` | `cli/record.go` | Читает JSONL-лог и выводит агрегированный отчёт |

### Команда `serve` -- основной режим

Это самая важная команда. Вот что происходит при `ffuuzz serve`:

```
1. Загрузить конфигурацию (env → flags)               config.Load()
2. Подключиться к PostgreSQL, накатить миграции        db.Open()
3. Создать store-ы (recording, campaign, finding, artifact)
4. Создать corpus.Manager                              corpus.NewManager()
5. Создать Engine                                      engine.NewEngine()
6. Создать endpoint.Resolver + восстановить тrie из БД resolver.RebuildFromDB()
7. Создать DBRecorder (запись трафика в БД)             recorder.NewDBRecorder()
8. Создать CertStore (Root CA + кеш сертификатов)      store.NewCertStore()
9. Создать MITM-прокси                                 mitm.New()
10. Запустить прокси в горутине                         proxy.ListenAndServe()
11. Подготовить embedded SPA                            fs.Sub(web.DistFS, "dist")
12. Создать API-сервер с Gin                            api.NewServer()
13. Запустить API в горутине                            apiSrv.ListenAndServe()
14. Запустить фоновый reproduce worker                  eng.StartReproduceWorker()
15. Ожидать SIGINT/SIGTERM → graceful shutdown
```

---

## 6. Сценарий 1: Запись трафика через MITM-прокси

> *Пользователь настраивает браузер на HTTP-прокси `localhost:8080`
> и ходит по тестируемому приложению. Все запросы записываются.*

### Путь запроса через код

```
Браузер → :8080 → mitm.Proxy.ServeHTTP()
                    │
                    ├── HTTP-запрос → handleHTTP()
                    └── CONNECT (HTTPS) → handleCONNECT() → mitmHTTPS() → handleHTTP()
```

**Файл**: `internal/mitm/mitm.go`

#### Шаг 1: Перехват

Метод `ServeHTTP()` проверяет: если это `CONNECT` -- значит HTTPS, иначе -- обычный HTTP.

#### Шаг 2: Для HTTPS -- TLS MITM

`handleCONNECT()` делает «hijack» соединения (забирает сырой TCP-сокет),
отправляет клиенту `200 Connection established`, генерирует leaf-сертификат
для домена через `CertStore.GetCertFor(host)` и создаёт TLS-соединение.
После этого запросы внутри TLS-тоннеля обрабатываются как обычный HTTP.

**Файл**: `internal/store/store.go`

`CertStore` -- управление сертификатами:
- При первом запуске генерирует **Root CA** (или загружает из `certs/`).
- Для каждого нового домена генерирует **leaf-сертификат**, подписанный Root CA.
- Сертификаты кешируются в LRU-кеше (по умолчанию 1000 записей).
- `singleflight` предотвращает дублирование генерации для одного хоста.

#### Шаг 3: Проксирование и запись

`handleHTTP()`:
1. Клонирует запрос, удаляет hop-by-hop заголовки.
2. Оборачивает тело запроса в `TeeReadCloser` -- чтобы прочитать тело и
   одновременно переслать его upstream.
3. Выполняет `transport.RoundTrip()` -- отправляет запрос на реальный сервер.
4. Копирует ответ клиенту, параллельно записывая тело ответа.
5. Формирует `TxRecord` и вызывает `recorder.Record()`.

#### Шаг 4: Сохранение в БД

**Файл**: `internal/recorder/recorder.go`

`DBRecorder.Record()`:
1. Парсит URL, извлекает scheme/host/port/path.
2. **Нормализует путь** (заменяет UUID, числа, хеши на `{_}`).
3. Передаёт путь в `Resolver.ObservePath()` для статистического схлопывания.
4. Формирует `RecordingSession` с одним `Exchange`.
5. Вызывает `store.FindOrAppend()` -- если запись для этого эндпоинта уже
   существует, exchange добавляется к ней; иначе создаётся новая запись.

---

## 7. Сценарий 2: Создание и запуск кампании фаззинга

> *Пользователь через Web UI (или REST API) создаёт кампанию, выбирает
> записи-сиды, настраивает параметры и нажимает «Start».*

### Создание кампании

**API**: `POST /api/v1/campaigns`

**Файл**: `internal/api/campaigns.go` → `createCampaign()`

1. Принимает JSON с именем, списком recording_ids и конфигурацией.
2. Проверяет, что все recording_ids существуют в БД.
3. Если `target.base_url` не задан -- выводит его из первой записи.
4. Валидирует конфигурацию (`validateCampaignConfig()`).
5. Сохраняет кампанию со статусом `CREATED`.

### Запуск кампании

**API**: `POST /api/v1/campaigns/:id/start`

**Файл**: `internal/api/campaigns.go` → `startCampaign()`

1. Проверяет, что кампания в допустимом статусе (CREATED, STOPPED, FINISHED, FAILED).
2. Вызывает `engine.StartCampaign()`.

**Файл**: `internal/engine/engine.go` → `StartCampaign()`

```
CREATED → STARTING (status update в БД)
    ↓
Загрузка сидов: corpus.GetSeeds()
    ↓
Расчёт baseline-ов: corpus.ComputeBaseline()
    ↓
STARTING → RUNNING (status update)
    ↓
go runCampaign()  ← запускается в отдельной горутине
```

### Горутина `runCampaign()`

Вот что происходит внутри:

```
1. Создать Pipeline мутаций       mutate.NewPipeline(cfg)
2. Создать MultiDetector          anomaly.NewMultiDetector(cfg)
3. Создать Triager                triage.NewTriager()
4. Создать Replayer               replayer.New()
5. Создать rate-limiter           NewLimiter(rps)
6. Запустить N воркеров            → Worker.Run() (горутины)
7. Генератор задач (main loop):
   while (не достигнут max_tests/duration/cancel):
       rate_limit → pick_random_seed → send SeedTask to channel
8. Закрыть канал задач, дождаться воркеров
9. Установить финальный статус (FINISHED или STOPPED)
```

---

## 8. Сценарий 3: Мутация запросов

> *Каждый воркер получает задачу (SeedTask), мутирует записанный трафик
> и отправляет его на цель.*

**Файлы**: `internal/mutate/`

### Pipeline

`Pipeline.Mutate()` в `mutate/mutate.go` -- это конвейер мутаторов.
Для каждого exchange он последовательно (с вероятностью `intensity`) применяет:

| Мутатор | Файл | Что делает |
|---|---|---|
| `URIMutator` | `mutate/uri.go` | Вставка/удаление сегментов пути, мутация query-параметров, инъекция спецсимволов, невалидные percent-encodings, манипуляции со слешами |
| `HeaderMutator` | `mutate/header.go` | Добавление/удаление/мутация HTTP-заголовков |
| `JSONMutator` | `mutate/json.go` | Мутация полей JSON-тел: смена типов, пустые значения, вложенные объекты |
| `ParamMutator` | `mutate/param.go` | Мутация form-параметров |
| `PrimitiveMutator` | `mutate/primitive.go` | Fallback: bit-flip, byte-shuffle, вставка нулевых байтов |
| `SeqMutator` | `mutate/sequence.go` | Мутация последовательности exchange-ов: перестановка, удаление, дублирование |

### Примеры мутаций URI

```
Исходный:  GET /api/users/42?page=1
Мутации:
  uri:path_segment    → /api/xKq3w/users/42?page=1      (вставка сегмента)
  uri:query_param     → /api/users/42?page=1&fuzz=AAAA.. (длинный параметр)
  uri:reserved_inject → /api/users/42?page=1#extra       (инъекция #)
  uri:slash_manip     → /api/users/42/..?page=1          (dot-segment)
```

### Intensity

Параметр `intensity` (0.0 .. 1.0) контролирует вероятность применения
каждого мутатора. При `intensity = 0.5` каждый мутатор в пайплайне
применяется с вероятностью 50%. Если ни один не сработал, применяется
`PrimitiveMutator` как fallback.

### Ограничения размера

После мутации `enforceSizeLimits()` обрезает URL (8 KB), заголовки (8 KB)
и тело (1 MB), чтобы мутации не породили гигантские запросы.

---

## 9. Сценарий 4: Повтор (replay) и обнаружение аномалий

> *Мутированные запросы отправляются на тестируемое приложение.
> Ответы анализируются на предмет аномалий.*

### Worker.processTask()

**Файл**: `internal/engine/worker.go`

```
SeedTask {Session, MutationSeed}
    ↓
[Sequence mutation] ← если включена
    ↓
[Per-exchange mutation] ← Pipeline.Mutate() для каждого exchange
    ↓
replayer.ReplaySession() ← отправка на цель
    ↓
Для каждого результата:
    detector.Detect() ← проверка аномалий
    ↓
Если найдена аномалия → handleHit()
```

### Replayer

**Файл**: `internal/replayer/replayer.go`

`ReplaySession()` отправляет exchange-ы *последовательно* (чтобы
сохранить зависимости между запросами). Для каждого exchange:

1. Применяет подстановки переменных (`{{token}}` → реальное значение).
2. Формирует `http.Request` из `Exchange`.
3. Отправляет через `http.Client` с таймаутом.
4. Собирает результат: status, headers, body, duration, error.
5. Извлекает переменные из ответа (если заданы `ExtractionRules`).

### WorkerContext -- stateful replay

**Файл**: `internal/replayer/context.go`

Каждый воркер имеет изолированный `WorkerContext`:
- **CookieJar** -- куки переносятся между запросами внутри одной сессии.
- **Variables** -- переменные, извлечённые из ответов через regex.
- **Client** -- свой `http.Client` с собственным jar и таймаутом.

Это позволяет тестировать stateful API (например, login → получить token →
использовать token в следующих запросах).

### Детекторы аномалий

**Файл**: `internal/anomaly/detector.go`

`MultiDetector` запускает все включённые детекторы для каждого результата:

| Детектор | Когда срабатывает |
|---|---|
| `TimeoutDetector` | Запрос завершился по таймауту (всегда включён) |
| `ServerErrorDetector` | HTTP-статус >= 500 И baseline не был 5xx |
| `LatencyDetector` | Время ответа > baseline_p50 * multiplier |
| `RegexDetector` | Тело ответа матчит один из заданных regex-паттернов |

### Дедупликация

При обнаружении аномалии воркер:
1. Вычисляет **сигнатуру**: `TYPE|METHOD|normalizedPath|hash(payload)`.
2. Проверяет через `findings.ExistsBySignature()`, нет ли уже такой находки.
3. Если нет -- создаёт `Finding` и записывает `Artifact` на диск.

---

## 10. Сценарий 5: Триаж -- подтверждение и минимизация находок

> *Когда найдена аномалия, система пытается её подтвердить (перепроверить)
> и минимизировать (убрать лишнее из запроса).*

**Файл**: `internal/triage/triage.go`

### Подтверждение (Confirm)

Если `triage.confirm_runs > 0`:
1. Повторить сессию N раз.
2. Каждый раз проверить, срабатывает ли детектор.
3. Если аномалия воспроизвелась в >= 50% прогонов → `CONFIRMED`.

### Минимизация сессии (MinimizeSession)

Если `triage.enable_minimization` И находка подтверждена:

**Фаза 1**: Удаление лишних exchange-ов.
- Перебираем exchange-ы с конца к началу.
- Пробуем убрать каждый (кроме первого).
- Если аномалия всё ещё срабатывает без этого exchange-а -- убираем его.

**Фаза 2**: Минимизация JSON-тел (`MinimizeJSONBody`).
- Для каждого оставшегося exchange-а с JSON-телом:
- Алгоритм **delta debugging** -- бинарный поиск по ключам JSON:
  - Попробовать оставить только правую половину ключей.
  - Попробовать оставить только левую.
  - Если ни одна половина не работает -- рекурсивно разбивать дальше.
  - Рекурсия во вложенные объекты (до глубины 5).

Результат: минимальный набор запросов и полей, который всё ещё
вызывает аномалию.

---

## 11. Сценарий 6: Воспроизведение находки по запросу (Reproduce)

> *Пользователь нажимает «Reproduce» в UI для конкретной находки.*

**API**: `POST /api/v1/findings/:id/reproduce`

**Файл**: `internal/api/findings.go` → `reproduceFinding()`

1. Устанавливает `reproduce_status = ENQUEUED` в БД.
2. Возвращает `202 Accepted`.

### Фоновый ReproduceWorker

**Файл**: `internal/engine/reproduce.go`

Запускается при старте сервера (`eng.StartReproduceWorker()`).
Каждые 5 секунд опрашивает БД через `ClaimNextReproduceJob()`.

Когда находит задачу:
1. Загружает `Finding` и `Artifact` из БД.
2. Читает JSON-файл артефакта с диска.
3. Создаёт `MultiDetector`, соответствующий типу находки.
4. Повторяет сессию N раз.
5. Если аномалия воспроизвелась в >= 50% прогонов → `CONFIRMED`,
   иначе → `NOT_REPRODUCED`.

---

## 12. Сценарий 7: Работа с веб-интерфейсом и SSE-стримингом

> *Пользователь открывает `http://localhost:8081` в браузере и наблюдает
> за ходом кампании в реальном времени.*

### SPA (Single Page Application)

**Файл**: `web/embed.go`

Фронтенд собирается Vite в `web/dist/` и встраивается в Go-бинарник
через `embed.FS`. При запуске `serve` он монтируется на `/ui/*`.

### REST API

**Файл**: `internal/api/server.go`

Все маршруты зарегистрированы в `NewServer()`:

```
/healthz                            GET     -- проверка здоровья
/metrics                            GET     -- Prometheus-метрики

/api/v1/recordings                  GET     -- список записей
/api/v1/recordings/:id              GET     -- детали записи
/api/v1/recordings/import           POST    -- импорт записей
/api/v1/recordings/export           GET     -- экспорт записей
/api/v1/recordings/tree             GET     -- древовидная структура записей
/api/v1/recordings/:id              DELETE  -- удалить запись
/api/v1/recordings/by-prefix        DELETE  -- массовое удаление

/api/v1/campaigns                   POST    -- создать кампанию
/api/v1/campaigns                   GET     -- список кампаний
/api/v1/campaigns/:id               GET     -- детали кампании
/api/v1/campaigns/:id/config        GET     -- конфигурация кампании
/api/v1/campaigns/:id/stats         GET     -- статистика кампании
/api/v1/campaigns/:id/findings      GET     -- находки кампании
/api/v1/campaigns/:id/stream        GET     -- SSE-стрим статистики
/api/v1/campaigns/:id/start         POST    -- запустить кампанию
/api/v1/campaigns/:id/stop          POST    -- остановить кампанию
/api/v1/campaigns/:id/recordings    POST    -- добавить записи к кампании

/api/v1/findings                    GET     -- список находок (с фильтрами)
/api/v1/findings/:id                GET     -- детали находки
/api/v1/findings/:id/artifact       GET     -- артефакт находки
/api/v1/findings/:id/reproduce      POST    -- запросить воспроизведение
```

### SSE-стриминг статистики

**Файл**: `internal/api/sse.go`

`GET /api/v1/campaigns/:id/stream`:
1. Устанавливает SSE-заголовки (`Content-Type: text/event-stream`).
2. Каждые 2 секунды собирает `CampaignStats` и отправляет как `data: {...}\n\n`.
3. Когда кампания завершается (FINISHED/STOPPED/FAILED), отправляет
   `event: done\ndata: {...}\n\n` и закрывает соединение.

---

## 13. Как данные хранятся: слой базы данных

**Файлы**: `internal/db/`

### Схема БД (упрощённо)

```
recordings (id, scheme, host, port, path, entry_count, ...)
    ↓ 1:N
exchanges (recording_id, method, path, query, headers, body, status, ...)

campaigns (id, name, status, config JSONB, tests_done, findings_total, ...)
    ↓ M:N
campaign_recordings (campaign_id, recording_id)

findings (id, campaign_id, type, status, signature, method, endpoint, details JSONB, ...)
    ↓ 1:1
artifacts (id, finding_id, file_path, size_bytes, ...)
```

### Миграции

Миграции хранятся в `internal/db/migrations/` и встроены через `embed.FS`.
При запуске `db.Open()` автоматически применяет все неприменённые миграции
через `golang-migrate`.

### Store-ы

Для каждой таблицы есть свой «store» -- файл с SQL-операциями:

| Store | Файл | Таблица |
|---|---|---|
| `RecordingStore` | `db/recordings.go` | recordings + exchanges |
| `CampaignStore` | `db/campaigns.go` | campaigns + campaign_recordings |
| `FindingStore` | `db/findings.go` | findings |
| `ArtifactStore` | `db/artifacts.go` | artifacts |

Все store-ы работают через `sqlx` и принимают `context.Context` для
поддержки отмены и таймаутов.

---

## 14. Нормализация эндпоинтов и статистическое схлопывание

> *Как `/api/users/42` и `/api/users/999` становятся одним эндпоинтом
> `/api/users/{_}`?*

Это двухфазный процесс.

### Фаза 1: Эвристическая нормализация

**Файл**: `internal/endpoint/normalize.go`

`NormalizePath()` заменяет сегменты, которые *выглядят* как параметры:

| Паттерн | Пример | Результат |
|---|---|---|
| Числовой ID | `/users/42` | `/users/{_}` |
| UUID | `/items/550e8400-e29b-41d4-a716-446655440000` | `/items/{_}` |
| Hex-хеш >= 8 символов | `/assets/a1b2c3d4e5f6` | `/assets/{_}` |
| Content-hashed файл | `/app.a1b2c3d4.js` | `/{_}` |
| Высокоэнтропийный токен >= 16 символов | `/session/eyJhbGciOiJIUzI1NiJ9` | `/session/{_}` |

### Фаза 2: Статистическое схлопывание (Resolver)

**Файлы**: `internal/endpoint/resolver.go`, `internal/endpoint/trie.go`

Работает с trie-деревом (префиксное дерево) для каждого origin:

1. Каждый наблюдаемый путь вставляется в trie.
2. Если на одном уровне trie количество уникальных сегментов превышает
   порог -- это «схлопывание» (collapse): все сегменты заменяются на `{_}`.
3. Записи в БД обновляются асинхронно через `MergeRecordings()`.
4. При старте сервера trie восстанавливается из БД (`RebuildFromDB()`).

Пример: если прокси видит `/api/products/shoes`, `/api/products/hats`,
`/api/products/bags`, ..., `/api/products/socks` -- при достижении порога
все они схлопываются в `/api/products/{_}`.

---

## 15. Метрики и мониторинг

**Файл**: `internal/metrics/metrics.go`

Все метрики регистрируются в отдельном `prometheus.Registry` (не
используется дефолтный глобальный):

| Метрика | Тип | Описание |
|---|---|---|
| `ffuuzz_tests_total` | Counter | Всего выполнено фазз-тестов |
| `ffuuzz_findings_total{type}` | CounterVec | Находки по типу |
| `ffuuzz_request_duration_seconds` | Histogram | Время запросов через прокси |
| `ffuuzz_corpus_size` | Gauge | Текущее количество записей |
| `ffuuzz_cert_cache_hits_total` | Counter | Попадания в кеш сертификатов |
| `ffuuzz_cert_cache_misses_total` | Counter | Промахи кеша сертификатов |
| `ffuuzz_cert_cache_evictions_total` | Counter | Вытеснения из кеша |
| `ffuuzz_connect_errors_total{class}` | CounterVec | Ошибки CONNECT по классу |
| `ffuuzz_cert_errors_total` | Counter | Ошибки генерации сертификатов |
| `ffuuzz_endpoint_collapses_total` | Counter | Статистические схлопывания |
| `ffuuzz_endpoint_merges_total` | Counter | Мерджи записей после схлопывания |

Метрики доступны по `GET /metrics`.

---

## 16. Конфигурация приложения

**Файл**: `internal/config/config.go`

Конфигурация загружается в два этапа:

1. **Значения по умолчанию** → `DefaultConfig()`
2. **Переменные окружения** (префикс `FFUUZZ_`) перезаписывают дефолты.
3. **CLI-флаги** перезаписывают всё остальное.

| Параметр | Env | Flag | Дефолт |
|---|---|---|---|
| API-адрес | `FFUUZZ_API_ADDRESS` | `-a` | `:8081` |
| Прокси-адрес | `FFUUZZ_PROXY_ADDRESS` | `-p` | `:8080` |
| PostgreSQL URI | `FFUUZZ_DATABASE_URI` | `-d` | `postgres://ffuuzz:ffuuzz@localhost:5432/ffuuzz?sslmode=disable` |
| Папка артефактов | `FFUUZZ_ARTIFACT_DIR` | `-o` | `./artifacts` |
| Таймаут запросов | `FFUUZZ_REQ_TIMEOUT` | -- | `3s` |
| Таймаут shutdown | `FFUUZZ_SHUTDOWN_TIMEOUT` | -- | `30s` |
| Воркеры | `FFUUZZ_WORKERS` | -- | `8` |
| RPS | `FFUUZZ_RPS` | -- | `50` |
| Макс. тело запроса | -- | `-max-body` | `64 KB` |
| Пропускать TLS | `FFUUZZ_TLS_SKIP_VERIFY` | `-tls-skip-verify` | `true` |

---

## 17. Жизненный цикл: запуск и грациозная остановка

**Файл**: `internal/cli/serve.go`

### Запуск

```
config.Load() → db.Open() → создание зависимостей →
proxy.ListenAndServe() (горутина) →
apiSrv.ListenAndServe() (горутина) →
eng.StartReproduceWorker() →
ожидание сигнала (SIGINT/SIGTERM)
```

### Остановка (graceful shutdown)

```
Получен SIGINT/SIGTERM
    ↓
1. apiSrv.Shutdown()           ← перестать принимать новые запросы, дождать in-flight
    ↓
2. eng.StopAll()               ← отменить все кампании + дождать reproduce worker
    ↓
3. proxy.Shutdown()            ← остановить прокси
    ↓
4. database.Close()            ← закрыть соединение с БД (defer)
```

Таймаут на весь shutdown: 30 секунд (настраивается).

---

## 18. Фронтенд (web/)

Фронтенд -- это React SPA, собранный с помощью Vite. Используются:

- **React** -- UI-библиотека.
- **TailwindCSS + DaisyUI** -- стилизация (utility-first + компонентная).
- **TanStack Query** -- управление серверным состоянием (кеширование, рефетч).

Фронтенд собирается в `web/dist/` командой `npm run build` и
встраивается в Go-бинарник через `embed.FS`. Доступен по `/ui/`.

При разработке можно запустить `make dev-frontend` для HMR (Hot Module
Replacement) -- фронтенд будет обслуживаться Vite на отдельном порту.

---

## 19. Шпаргалка: где что искать

| Хочу разобраться в... | Смотрю в... |
|---|---|
| Структурах данных | `internal/model/model.go` |
| Как запускается приложение | `cmd/ffuuzz/main.go` → `internal/cli/serve.go` |
| Как работает прокси | `internal/mitm/mitm.go` |
| Как записывается трафик | `internal/recorder/recorder.go` |
| Как мутируются запросы | `internal/mutate/mutate.go` + `uri.go`, `json.go`, etc. |
| Как отправляются мутации | `internal/replayer/replayer.go` |
| Как обнаруживаются аномалии | `internal/anomaly/detector.go` |
| Как подтверждаются находки | `internal/triage/triage.go` |
| Как оркестрируется кампания | `internal/engine/engine.go` + `worker.go` |
| Как работает REST API | `internal/api/server.go` (маршруты) → `campaigns.go`, `recordings.go`, `findings.go` |
| Как хранятся данные | `internal/db/` (stores) + `internal/db/migrations/` (схема) |
| Как нормализуются эндпоинты | `internal/endpoint/normalize.go` + `resolver.go` + `trie.go` |
| Как генерируются сертификаты | `internal/store/store.go` |
| Какие метрики экспортируются | `internal/metrics/metrics.go` |
| Как устроен фронтенд | `web/` |
