# L7-фаззер прикладного уровня «FFUZZ» (на базе MITM-прокси + рекордера)

Документ описывает требования к итоговой командной работе (2 человека). Результат: реализованный прототип FFUZZ и сопровождающая документация/тесты.

---

## 1. Термины и сокращения

- **FFUZZ** — разрабатываемая система (MITM-прокси, рекордер, движок фаззинга, Control API, хранилище).

- **Фаззинг** — метод автоматизированного тестирования, при котором приложению подаются (в том числе) случайные и/или некорректные входные данные для выявления сбоев и аномалий.

- **SUT (System Under Test)** — целевое тестируемое приложение.

- **Сессия (RecordingSession)** — записанная последовательность HTTP-обменов request/response, полученная рекордером.

- **Обмен (Exchange)** — один request/response внутри сессии.

- **Корпус (Corpus)** — набор seed-сессий, используемых движком для генерации тестов.

- **Кампания (Campaign)** — конфигурация и процесс выполнения фаззинга.

- **Находка (Finding)** — зафиксированная аномалия (например, timeout/5xx/latency regression/regex match) + артефакт воспроизведения.

- **Артефакт (Artifact)** — сериализованное описание, достаточное для воспроизведения находки.

---

## 2. Общие требования

Система FFUZZ предназначена для фазз-тестирования HTTP/HTTPS-сервисов на L7 в локальном/тестовом контуре.

FFUZZ должна обеспечивать:

1. перехват HTTP/HTTPS (MITM) и запись трафика в виде сессий;
   > 📎 **Реализация:** [mitm.go](internal/mitm/mitm.go) — HTTP-проксирование; [mitm.go](internal/mitm/mitm.go) — CONNECT-туннелирование и MITM TLS; [recorder.go](internal/recorder/recorder.go) — запись TxRecord в JSONL.

2. импорт ранее записанных сессий;
   > 📎 **Реализация:** [recordings.go](internal/api/recordings.go) — хендлер `importRecordings` (POST /api/v1/recordings/import); [recordings.go](internal/db/recordings.go) — `RecordingStore.Upsert` (идемпотентная вставка).

3. формирование корпуса из записанных и импортированных сессий;
   > 📎 **Реализация:** [corpus.go](internal/corpus/corpus.go) — `Manager.GetSeeds()` загружает сессии, `ComputeBaseline()` вычисляет baseline по записям.

4. запуск кампаний фаззинга: генерация и мутация запросов (в том числе внутри последовательностей), отправка в SUT, сбор ответов;
   > 📎 **Реализация:** [engine.go](internal/engine/engine.go) — `StartCampaign` / `runCampaign`; [worker.go](internal/engine/worker.go) — цикл воркера (мутация → реплей → детект); пакет `internal/mutate/` — все мутаторы; [replayer.go](internal/replayer/replayer.go) — `ReplaySession` / `ReplayExchange`.

5. детект аномалий (как минимум: 5xx, таймаут, деградация latency относительно baseline);
   > 📎 **Реализация:** [detector.go](internal/anomaly/detector.go) — `TimeoutDetector`, `ServerErrorDetector`, `LatencyDetector`, `RegexDetector`, `MultiDetector`.

6. triage: дедупликация находок, подтверждение воспроизведением, минимизация воспроизводящего тест-кейса;
   > 📎 **Реализация:** [triage.go](internal/triage/triage.go) — `Signature()` (дедупликация), `Confirm()` (N повторов), `MinimizeSession()` (бинарный поиск обменов), `MinimizeJSONBody()` (delta-debug JSON-полей).

7. хранение метаданных кампаний/корпуса/находок в БД и хранение артефактов воспроизведения;
   > 📎 **Реализация:** `internal/db/` — [recordings.go](internal/db/recordings.go), [campaigns.go](internal/db/campaigns.go), [findings.go](internal/db/findings.go), [artifacts.go](internal/db/artifacts.go) (PostgreSQL); [001_initial.up.sql](internal/db/migrations/001_initial.up.sql) — миграции; артефакты на диске через `ArtifactDir` ([worker.go](internal/engine/worker.go) — `writeArtifact`).

8. HTTP Control API для управления и выгрузки результатов;
   > 📎 **Реализация:** пакет `internal/api/` — [server.go](internal/api/server.go) (Gin-маршрутизация, middleware), [recordings.go](internal/api/recordings.go), [campaigns.go](internal/api/campaigns.go), [findings.go](internal/api/findings.go), [campaign_config.go](internal/api/campaign_config.go), [sse.go](internal/api/sse.go).

9. метрики.
   > 📎 **Реализация:** [metrics.go](internal/metrics/metrics.go) — `ffuuzz_tests_total`, `ffuuzz_findings_total{type}`, `ffuuzz_request_duration_seconds`, `ffuuzz_corpus_size`, LRU-метрики кэша, `ffuuzz_connect_errors_total`, `ffuuzz_cert_errors_total`; эндпоинт `/metrics` в [server.go](internal/api/server.go).

---

## 3. Абстрактная схема взаимодействия с системой

1. Пользователь поднимает целевое приложение (SUT) в тестовом окружении.

2. Пользователь запускает FFUZZ (прокси, рекордер, движок, Control API).

3. Пользователь устанавливает корневой сертификат FFUZZ CA (только для тестового окружения) и настраивает клиент (браузер/curl) на FFUZZ как на прокси.

4. Пользователь выполняет тестируемые сценарии (логин/навигация/действия), FFUZZ записывает один или несколько наборов сессий (seeds).

5. Пользователь создаёт кампанию, выбирает набор seed-сессий и параметры фаззинга.

6. FFUZZ выполняет кампанию, собирает статистику и находки.

7. Для находок FFUZZ сохраняет артефакты воспроизведения, выполняет подтверждение и минимизацию.

8. Пользователь получает результаты через Control API.

---

## 4. Целевые приложения (SUT) — внешняя зависимость

Разработка уязвимого приложения в рамках командной работы не требуется.

Требование: обеспечить воспроизводимый запуск минимум одного SUT в Docker (docker-compose или эквивалент) и описать шаги запуска в README.

Рекомендуемый SUT для демонстрации:

- **[GoVWA (Go Vulnerable Web Application)](https://github.com/0c34/govwa)** — ориентирован на локальный запуск и наличие многошаговых сценариев.

Дополнительный SUT (опционально):

- **[damn-vulnerable-golang](https://github.com/TheHackerDev/damn-vulnerable-golang)** — допускается использовать только внутри изолированного контейнера; запуск на хосте запрещён (соображения безопасности).

---

## 5. Состав системы FFUZZ

Система состоит из логических компонентов (может быть реализовано одним бинарём):

1. **MITM-прокси**  
   Принимает HTTP и CONNECT (HTTPS), выполняет MITM-терминацию TLS и форвардит трафик.
   > 📎 **Реализация:** [mitm.go](internal/mitm/mitm.go) — `Proxy` struct; HTTP: `handleHTTP()`; CONNECT: `handleCONNECT()`; MITM TLS: `mitmHTTPS()`.

2. **Рекордер**  
   Сохраняет перехваченные обмены в формате RecordingSession (см. раздел 8). Поддерживает два режима работы: `JSONLRecorder` для записи в JSONL-файлы (команда `proxy`) и `DBRecorder` для прямой записи в PostgreSQL с группировкой по endpoint (команда `serve`). `DBRecorder` определяет endpoint по `scheme://host:port/path`, нормализует путь через пакет `endpoint` (замена UUID, числовых ID, хэшей на плейсхолдер `{_}`), и группирует обмены в одну RecordingSession per endpoint.
   > 📎 **Реализация:** [recorder.go](internal/recorder/recorder.go) — `Recorder` интерфейс, `JSONLRecorder` (запись TxRecord в JSONL), `DBRecorder` (запись в PostgreSQL через `RecordingInserter.FindOrAppend()`); конвертеры `TxRecordToExchange()` / `ExchangeToTxRecord()`.

3. **Менеджер корпуса**  
   Хранит и индексирует сессии и обеспечивает выборку seed-сессий для кампаний.
   > 📎 **Реализация:** [corpus.go](internal/corpus/corpus.go) — `Manager` struct; `GetSeeds()` — загрузка seed-сессий из БД; `ComputeBaseline()` — вычисление p50 latency per endpoint.

4. **Движок фаззинга (Fuzz Engine)**  
   Запускает кампании, мутирует запросы (в том числе stateful-последовательности), отправляет в SUT, собирает ответы и детектит аномалии.
   > 📎 **Реализация:** [engine.go](internal/engine/engine.go) — `Engine` struct, `StartCampaign`, `runCampaign`; [worker.go](internal/engine/worker.go) — `Worker.Run()` / `processTask()`; [ratelimit.go](internal/engine/ratelimit.go) — token-bucket RPS limiter; [reproduce.go](internal/engine/reproduce.go) — воспроизведение находок.

5. **Triage**  
   Дедупликация + подтверждение воспроизведением + минимизация.
   > 📎 **Реализация:** [triage.go](internal/triage/triage.go) — `Signature()`, `Confirm()`, `MinimizeSession()`, `MinimizeJSONBody()`.

6. **Control API**  
   REST API управления кампанией/корпусом и выгрузки результатов.
   > 📎 **Реализация:** [server.go](internal/api/server.go) — Gin-сервер с request ID middleware, logging middleware; хендлеры: [recordings.go](internal/api/recordings.go), [campaigns.go](internal/api/campaigns.go), [findings.go](internal/api/findings.go), [campaign_config.go](internal/api/campaign_config.go), [sse.go](internal/api/sse.go).

7. **Storage**  
   PostgreSQL для метаданных + файловое хранилище для артефактов.
   > 📎 **Реализация:** [db.go](internal/db/db.go) — `Database` (sqlx + миграции); [recordings.go](internal/db/recordings.go), [campaigns.go](internal/db/campaigns.go), [findings.go](internal/db/findings.go), [artifacts.go](internal/db/artifacts.go); миграции в `internal/db/migrations/`; артефакты на диске через `ArtifactDir`.

8. **Traffic Diff**  
   Сравнение пар TxRecord для выявления расхождений в URL и статусах ответов.
   > 📎 **Реализация:** [diff.go](internal/diff/diff.go) — `TxDiff` struct, `DiffTxRecords()` — сравнение URL и статуса ответа.

9. **Report**  
   Построение агрегированной сводки (по методам, статусам, хостам) по записанным TxRecord.
   > 📎 **Реализация:** [report.go](internal/report/report.go) — `Summary` struct, `BuildSummary()` — агрегация по method/status/host.

10. **Web UI (SPA)**  
    Встроенный веб-интерфейс, раздаваемый по маршруту `/ui/*`. Собранные ассеты SPA встраиваются в бинарь через `embed.FS`. При обращении к `/` выполняется редирект на `/ui/`. Для статических ассетов (`assets/`) устанавливается длительный кэш (`max-age=31536000, immutable`), для `index.html` — `no-cache`. Поддерживается client-side routing (fallback на `index.html`).
    > 📎 **Реализация:** [embed.go](web/embed.go) — `//go:embed all:dist/*`, `var DistFS embed.FS`; [spa.go](internal/api/spa.go) — `spaHandler()`: fallback на `index.html`, кэш `max-age=31536000, immutable` для `assets/*`, `no-cache` для `index.html`; редирект `/` → `/ui/` в [server.go](internal/api/server.go). Фронтенд: React 19 + Vite + Tailwind/DaisyUI, маршрутизация в [router.tsx](web/src/router.tsx). Страницы: `web/src/pages/` — `DashboardPage`, `RecordingsPage`, `RecordingDetailPage`, `CampaignsPage`, `CampaignCreatePage`, `CampaignDetailPage`, `FindingsPage`, `FindingDetailPage`, `NotFoundPage`. Компоненты: `web/src/components/` — `Layout`, `EndpointTree`, `ExchangeViewer`, `BodyViewer`, `JsonViewer`, `HeadersTable`, `FindingsTable`, `StatsCard`, `StatusBadge`, `ImportDialog`, `ConfirmDialog`, `Pagination`, `TimeAgo`, `LoadingSpinner`, `EmptyState`, `ErrorAlert`. API-клиент: `web/src/api/` — `client.ts`, `recordings.ts`, `campaigns.ts`, `findings.ts`, `health.ts`. Хуки: `web/src/hooks/` — `queries.ts` (React Query), `useSSE.ts` (SSE-стрим). Типы: [api.ts](web/src/types/api.ts).

11. **Endpoint Resolver**  
    Автоматическое обнаружение и нормализация endpoint-паттернов в записанном трафике. Система двухфазная: (1) эвристическая нормализация пути — замена UUID, числовых ID, hex-хэшей, content-hashed файлов и токенов на универсальный плейсхолдер `{_}`; (2) статистический коллапс — сегментный trie отслеживает кардинальность на каждом уровне пути и при превышении порога объединяет записи с разными значениями сегмента в один параметризованный endpoint. При коллапсе выполняется асинхронный merge записей в БД (перенос exchanges, удаление дубликатов).
    > 📎 **Реализация:** [normalize.go](internal/endpoint/normalize.go) — `NormalizePath()`, `isParameter()` (UUID, numeric, hex, hashed file, token); [trie.go](internal/endpoint/trie.go) — сегментный trie, `observe()`, `checkCollapse()`, `collapse()`; [resolver.go](internal/endpoint/resolver.go) — `Resolver` struct, `ObservePath()`, `RebuildFromDB()`, async `executeMerge()`, `drainCollapses()`.

---

## 5.1. CLI-команды

Исполняемый бинарь `ffuuzz` поддерживает следующие подкоманды:

- **`serve`** — запуск полной системы (прокси + Control API + движок). Основной production-режим. В этом режиме используется `DBRecorder` для прямой записи в PostgreSQL с endpoint-группировкой и `endpoint.Resolver` для статистического обнаружения паттернов.
  > 📎 **Реализация:** [serve.go](internal/cli/serve.go) — `runServe()`: инициализация DB, миграции, store'ы, corpus manager, engine, endpoint.Resolver (RebuildFromDB), DBRecorder, MITM proxy, API server ([server.go](internal/httputil/server.go) — `NewHTTPServer()`), graceful shutdown.

- **`proxy`** — запуск только MITM-прокси (dev-режим для отладки и записи трафика без БД и API).
  > 📎 **Реализация:** [proxy.go](internal/cli/proxy.go) — `runProxy()`: standalone MITM-прокси с записью в JSONL, CLI-флаги (port, output, cert dir, max body).

- **`record`** — анализ записанного JSONL-лога: парсинг TxRecord, построение сводки (report).
  > 📎 **Реализация:** [record.go](internal/cli/record.go) — `runRecord()`: чтение TxRecord из JSONL, `report.BuildSummary()`, вывод JSON в stdout.

---

## 6. Общие ограничения и требования к реализации

- Язык реализации: Go.
  > 📎 **Реализация:** [go.mod](go.mod) — `module ffuuzz`, Go.

- Хранилище метаданных: PostgreSQL (структура таблиц на усмотрение команды).
  > 📎 **Реализация:** [db.go](internal/db/db.go) — sqlx + PostgreSQL; таблицы: recordings, exchanges, campaigns, campaign_recordings, findings, artifacts ([001_initial.up.sql](internal/db/migrations/001_initial.up.sql)).

- Артефакты допускается хранить на диске; в БД хранить ссылки и метаданные.
  > 📎 **Реализация:** [artifacts.go](internal/db/artifacts.go) — метаданные (id, finding_id, file_path, size_bytes) в PostgreSQL; файлы пишутся в `ArtifactDir` ([worker.go](internal/engine/worker.go)).

- Конкурентность: кампания поддерживает параллельную отправку запросов и обработку ответов.
  > 📎 **Реализация:** [engine.go](internal/engine/engine.go) — пул из N воркеров (`cfg.Limits.Workers`, по умолчанию 4), taskCh канал задач.

- Лимиты: таймаут запроса, лимит RPS, max_tests и/или duration кампании.
  > 📎 **Реализация:** [model.go](internal/model/model.go) — `CampaignLimits` (Workers, RPS, MaxTests, DurationSec, ReqTimeoutMs); [ratelimit.go](internal/engine/ratelimit.go) — token-bucket RPS limiter; [engine.go](internal/engine/engine.go) — проверка maxTests/duration в цикле генерации.

- Корректное завершение всех компонентов по SIGINT/SIGTERM.
  > 📎 **Реализация:** [serve.go](internal/cli/serve.go) — `signal.NotifyContext(SIGINT, SIGTERM)`, последовательное завершение: API → Engine.StopAll → Proxy.

---

## 7. Требования к MITM-прокси и рекордеру

### 7.1. Корректное завершение работы (graceful shutdown)

Система должна обеспечивать корректное завершение работы прокси/рекордера по сигналам SIGINT/SIGTERM и/или по отмене контекста выполнения.

Требования:

1. При завершении прокси прекращает принимать новые входящие соединения и инициирует остановку фоновых задач.

2. Для уже установленных соединений прокси пытается завершить обработку текущих запросов/стримов и закрывает соединения.

3. Рекордер завершает запись всех уже полученных данных и обеспечивает консистентность формируемых сессий.

4. Должен существовать общий таймаут завершения (shutdown timeout): по истечении таймаута система принудительно закрывает оставшиеся соединения/ресурсы и завершает процесс. Значение по умолчанию — 30 секунд; должно быть конфигурируемым.

5. По завершении shutdown система пишет в лог итоговые счётчики (количество компонентов, закрытых штатно и принудительно).

> 📎 **Реализация (7.1):**
> - Пп. 1–2: [mitm.go](internal/mitm/mitm.go) — `Proxy.Shutdown(ctx)` делегирует в `http.Server.Shutdown`, который прекращает приём новых соединений и drain'ит in-flight запросы.
> - П. 3: рекордер `JSONLRecorder.Close()` ([recorder.go](internal/recorder/recorder.go)) — flush и закрытие файла; `DBRecorder.Close()` ([recorder.go](internal/recorder/recorder.go)) — завершение записей в PostgreSQL (режим `serve`).
> - П. 4: [config.go](internal/config/config.go) — `ShutdownTimeout`, default 30s; [main.go](cmd/ffuuzz/main.go) — `context.WithTimeout(cfg.ShutdownTimeout)`.
> - П. 5: [main.go](cmd/ffuuzz/main.go) — логирование `normal_close` / `forced_close` счётчиков при завершении. **Примечание:** считаются компоненты (API, прокси), а не отдельные TCP-соединения.

### 7.2. Уникальный Request ID и трассировка

Система должна присваивать каждому перехваченному HTTP-запросу уникальный Request ID и использовать его для сквозной трассировки.

Требования:

1. Request ID генерируется на каждый запрос и уникален во времени и в пределах процесса.

2. Формат Request ID сортируемый по времени и содержит:
   
   - префикс даты `YYYYMMDD`,
   
   - суффикс UUID v4.  
     Пример: `20260226-550e8400-e29b-41d4-a716-446655440000`.

3. Request ID:
   
   - добавляется в заголовок ответа `X-Request-ID`,
   
   - логируется во всех релевантных сообщениях,
   
   - сохраняется рекордером в составе Exchange.

> 📎 **Реализация (7.2):**
> - Генерация: [reqid.go](internal/httputil/reqid.go) — `NewRequestID()` = `YYYYMMDD-UUIDv4`.
> - X-Request-ID: [mitm.go](internal/mitm/mitm.go) — добавляется в ответ; [server.go](internal/api/server.go) — middleware для Control API.
> - Логирование: [logging.go](internal/logging/logging.go) — `WithRequestID()` добавляет поле `request_id` в zerolog.
> - Сохранение: [mitm.go](internal/mitm/mitm.go) — `TxRecord.RequestID`; [model.go](internal/model/model.go) — `Exchange.RequestID`.

### 7.3. Кэширование и вытеснение сертификатов (LRU)

Система должна кэшировать сгенерированные сертификаты для MITM TLS, чтобы уменьшать задержки и нагрузку при повторных соединениях на один и тот же hostname.

Требования:

1. Кэш сертификатов ограничен по размеру (количество записей). Значение по умолчанию — 1000; должно быть конфигурируемым.

2. При переполнении кэша применяется политика вытеснения LRU.

3. Должны быть метрики кэша: hits/misses/evictions.

4. Вытеснение из кэша не должно приводить к неконтролируемому росту потребления памяти.

> 📎 **Реализация (7.3):**
> - LRU-кэш: [store.go](internal/store/store.go) — `hashicorp/golang-lru/v2`, maxEntries default 1000, конфигурируется через [config.go](internal/config/config.go) `CertCache.MaxEntries` и CLI-флаг `-cert-cache-size`.
> - Метрики: [metrics.go](internal/metrics/metrics.go) — `ffuuzz_cert_cache_hits_total`, `ffuuzz_cert_cache_misses_total`, `ffuuzz_cert_cache_evictions_total`; инкрементируются в [store.go](internal/store/store.go).
> - Контроль памяти: eviction callback в [store.go](internal/store/store.go) при переполнении.

### 7.4. Усиление TLS и управляемая TLS-конфигурация

Система должна обеспечивать безопасную и управляемую TLS-терминацию для MITM-режима.

Требования:

1. Минимальная версия TLS для соединений «клиент ↔ прокси» должна быть не ниже TLS 1.2.

2. Должен существовать таймаут TLS-рукопожатия со значением по умолчанию 10 секунд; таймаут должен быть конфигурируемым.

3. Должна поддерживаться конфигурация допустимых наборов шифров (cipher suites) и/или режим «только безопасные наборы». При необходимости допускается отдельная конфигурация для направлений «клиент ↔ прокси» и «прокси ↔ upstream».

4. Должна быть опциональная возможность отключить TLS session tickets на стороне MITM-сервера.

5. TLS-параметры должны настраиваться через механизм конфигурирования системы (см. раздел 10).

> 📎 **Реализация (7.4):**
> - Min TLS 1.2: [store.go](internal/store/store.go) — `tls.VersionTLS12` default; [config.go](internal/config/config.go) `TLS.MinVersion`.
> - Handshake timeout 10s: [store.go](internal/store/store.go) default 10s; [mitm.go](internal/mitm/mitm.go) `SetDeadline()`.
> - Cipher suites: [store.go](internal/store/store.go) — настраиваемый список в `tls.Config`.
> - Session tickets: CLI `-tls-no-tickets` ([config.go](internal/config/config.go)); [store.go](internal/store/store.go) `SessionTicketsDisabled`.
> - Конфигурирование: [config.go](internal/config/config.go) — `TLSConfig` struct.

### 7.5. Обработка ошибок CONNECT и этапа hijack

Система должна корректно обрабатывать ошибки на этапе установления CONNECT-туннеля и/или перехода к проксированию байтового потока.

Требования:

1. Если на этапе CONNECT невозможно перейти к режиму туннелирования (ошибка hijack, ошибка записи/чтения, внутренняя ошибка), прокси:
   
   - возвращает клиенту HTTP-ответ 500 (если это возможно на данном этапе),
   
   - закрывает соединение,
   
   - фиксирует ошибку в логах вместе с Request ID и целевым host:port.

2. Должна быть метрика ошибок CONNECT/hijack (с категоризацией хотя бы по классу ошибки).

3. Опционально допускается реализовать ограниченный retry (например, 1–2 попытки) с backoff, если это не приводит к циклическим попыткам.

> 📎 **Реализация (7.5):**
> - HTTP 500 + закрытие: [mitm.go](internal/mitm/mitm.go) — `hijack_unsupported` и `hijack_failed` возвращают 500; также `write_200`, `cert_generation`, `tls_handshake`.
> - Логирование с Request ID и host:port: [mitm.go](internal/mitm/mitm.go).
> - Метрики по классу ошибки: [metrics.go](internal/metrics/metrics.go) — `ffuuzz_connect_errors_total{error_class}` с лейблами: `hijack_unsupported`, `hijack_failed`, `write_200`, `cert_generation`, `tls_handshake`.
> - Retry CONNECT: не реализован (опционально по спецификации).

### 7.6. Ошибки генерации и хранения сертификатов

Система не должна игнорировать ошибки при генерации, кэшировании и сохранении сертификатов.

Требования:

1. Любая ошибка генерации сертификата или записи на диск:
   
   - логируется уровнем не ниже WARNING,
   
   - увеличивает метрику ошибок,
   
   - корректно пробрасывается на уровень обработчика соединения.

2. Должен быть механизм retry генерации/сохранения сертификата с ограничением числа попыток.

3. Должен поддерживаться режим хранения сертификатов только в памяти без записи на диск.

4. При записи на диск операция должна быть атомарной и безопасной при конкурентном доступе.

> 📎 **Реализация (7.6):**
> - Логирование WARNING + метрика: [store.go](internal/store/store.go) — `CertErrors.Inc()` при генерации; [store.go](internal/store/store.go) — ошибки записи на диск; пробрасывается через [mitm.go](internal/mitm/mitm.go) `cert_generation`.
> - Retry 3 попытки: [store.go](internal/store/store.go) — цикл с backoff 10ms.
> - Memory-only режим: [config.go](internal/config/config.go) `CertCache.MemoryOnly` / CLI `-cert-memory-only`; [store.go](internal/store/store.go) — условие `memOnly`.
> - Атомарная запись: [store.go](internal/store/store.go) — `atomicWrite()`: temp file → `os.Rename`; потокобезопасность: `sync.Mutex` в `GetCertFor`.

---

## 8. Форматы данных

Все timestamp-поля должны быть в формате RFC3339. ([IETF Datatracker](https://datatracker.ietf.org/doc/html/rfc3339?utm_source=chatgpt.com "RFC 3339 - Date and Time on the Internet: Timestamps"))

Тела request/response передаются как `body_b64` (base64 от сырых байтов).

Headers представлены как `map[string][]string`.

`request_id` обязателен и должен соответствовать формату `YYYYMMDD-UUIDv4` (см. P1.2).

> 📎 **Реализация (8 общее):**
> - RFC3339 timestamps: `time.Time` с JSON-тегами throughout [model.go](internal/model/model.go); API парсит RFC3339 в `parseSinceParam()` ([server.go](internal/api/server.go)).
> - `body_b64`: [model.go](internal/model/model.go) — `BodyB64 string` + `BodyTruncated bool`; захват тела: [http.go](internal/httputil/http.go) — `LimitedBuffer` (ограничение размера тела), `TeeReadCloser` (чтение без потребления потока).
> - Headers `map[string][]string`: [model.go](internal/model/model.go); удаление hop-by-hop заголовков при проксировании: [http.go](internal/httputil/http.go) — `RemoveHopByHop()`, `CopyHeaders()`.
> - `request_id` YYYYMMDD-UUIDv4: [reqid.go](internal/httputil/reqid.go).

### 8.1. RecordingSession

> 📎 **Реализация:** [model.go](internal/model/model.go) — `RecordingSession` struct: `SchemaVersion`, `ID`, `CreatedAt`, `Target` (TargetInfo), `Entries` ([]Exchange), `EntryCount`.

`TargetInfo` содержит поля `scheme`, `host`, `port` и `path`. Поле `path` представляет нормализованный путь endpoint'а (с плейсхолдерами `{_}` для параметрических сегментов). При хранении в БД записи группируются по комбинации `(scheme, host, port, path)`.

```json
{
  "schema_version": 1,
  "id": "2a1f7c0d-0e7b-4f5b-9c4a-6c0a2f8b2b0e",
  "created_at": "2026-02-19T10:20:30Z",
  "target": {
    "scheme": "https",
    "host": "example.local",
    "port": 443,
    "path": "/api/users/{_}"
  },
  "entries": [ /* Exchange[] */ ]
}
```

### 8.2. Exchange

> 📎 **Реализация:** [model.go](internal/model/model.go) — `Exchange` struct: `RequestID`, `StartedAt`, `DurationMs`, `Request` (RequestData), `Response` (ResponseData); [model.go](internal/model/model.go) — `RequestData` / `ResponseData`.

```json
{
  "request_id": "20260219-7b4f2b86-4d7a-4d20-9e76-2fdc3b53d3f1",
  "started_at": "2026-02-19T10:20:31Z",
  "duration_ms": 120,
  "request": {
    "method": "POST",
    "path": "/login",
    "query": "a=1&b=2",
    "headers": {
      "Content-Type": ["application/json"]
    },
    "body_b64": "eyJ1c2VyIjoiYWRtaW4ifQ==",
    "body_truncated": false
  },
  "response": {
    "status": 200,
    "headers": {
      "Set-Cookie": ["sid=abc"]
    },
    "body_b64": "eyJvayI6dHJ1ZX0=",
    "body_truncated": false
  }
}
```

### 8.3. Единый формат ошибки Control API

> 📎 **Реализация:** [model.go](internal/model/model.go) — `APIError` struct (`error`, `message`, `request_id`); [server.go](internal/api/server.go) — `errorResponse()` / `internalError()`.

```json
{
  "error": "invalid_request",
  "message": "sessions[0].entries is required",
  "request_id": "20260219-..."
}
```

---

## 9. Требования к движку фаззинга и triage

### 9.1. Режим фаззинга: replay-based

Seed-входы берутся из корпуса (сессии рекордера). Для каждой seed-сессии:

- сохраняется baseline (оригинальная последовательность запросов/ответов);

- движок генерирует новые тесты путём мутаций отдельных запросов и/или операций над последовательностью (см. раздел 9.6).

> 📎 **Реализация (9.1):**
> - Seed-входы из корпуса: [corpus.go](internal/corpus/corpus.go) — `Manager.GetSeeds()`.
> - Baseline: [corpus.go](internal/corpus/corpus.go) — `ComputeBaseline()` → p50 latency per endpoint; [engine.go](internal/engine/engine.go).
> - Генерация тестов: [engine.go](internal/engine/engine.go) — цикл: случайный seed → `SeedTask{Session, MutationSeed}` → `taskCh`.

### 9.2. Stateful-контекст

Движок должен поддерживать:

- **cookie jar** (клиентское хранилище cookies): обработку `Set-Cookie` и автоматическую отправку `Cookie` в последующих запросах по правилам применимости cookies. ([IETF Datatracker](https://datatracker.ietf.org/doc/html/rfc6265?utm_source=chatgpt.com "RFC 6265 - HTTP State Management Mechanism"))

- переменные, извлечённые из ответов (минимум: regex или упрощённый JSONPath);

- подстановки в последующие запросы вида `{{var}}` в path/query/headers/body.

Требование к параллелизму stateful-контекста: cookie jar и набор переменных не должны смешиваться между независимыми параллельными цепочками выполнения (например, между воркерами кампании).

> 📎 **Реализация (9.2):**
> - Cookie jar: [context.go](internal/replayer/context.go) — `WorkerContext` с `CookieJar` (net/http/cookiejar), `UpdateCookies()`.
> - Извлечение переменных (regex): [context.go](internal/replayer/context.go) — `ExtractionRule` struct (Name, Source, Header, Regex), `ExtractVariables()`.
> - Подстановки `{{var}}`: [context.go](internal/replayer/context.go) — `ApplySubstitutions()` в path/query/headers/body.
> - Изоляция воркеров: [worker.go](internal/engine/worker.go) — `replayer.NewWorkerContext()` создаётся per-task (отдельный CookieJar + Variables).
> - Модель: [model.go](internal/model/model.go) — `ExtractionRule` struct.

### 9.3. Детект аномалий

Движок должен фиксировать находки типов:

- **TIMEOUT** — превышение таймаута запроса;

- **SERVER_ERROR** — ответ 5xx (опционально с учётом baseline);

- **LATENCY_REGRESSION** — превышение baseline_latency × K (K конфигурируемый);

- **REGEX_MATCH** — наличие в ответе строк, соответствующих заданному шаблону (набор шаблонов задаётся в конфигурации кампании).

> 📎 **Реализация (9.3):**
> - TIMEOUT: [detector.go](internal/anomaly/detector.go) — `TimeoutDetector` (DeadlineExceeded, net.Error.Timeout, os.IsTimeout).
> - SERVER_ERROR: [detector.go](internal/anomaly/detector.go) — `ServerErrorDetector` (5xx, с учётом baseline).
> - LATENCY_REGRESSION: [detector.go](internal/anomaly/detector.go) — `LatencyDetector` (baseline × multiplier).
> - REGEX_MATCH: [detector.go](internal/anomaly/detector.go) — `RegexDetector` (компиляция паттернов при старте, матч по body).
> - Композиция: [detector.go](internal/anomaly/detector.go) — `MultiDetector` составляет детекторы по `AnomalyConfig`.
> - Типы: [model.go](internal/model/model.go) — `FindingType` enum.

### 9.4. Triage

- Дедупликация: сигнатура = тип + нормализованный endpoint + хэш нормализованного payload (+ статус/класс ошибки, если применимо).

- Подтверждение: N повторов (N конфигурируемый) для статуса CONFIRMED/UNCONFIRMED.

- Минимизация: упрощение JSON body и (опционально) сокращение последовательности.

> 📎 **Реализация (9.4):**
> - Дедупликация: [triage.go](internal/triage/triage.go) — `Signature()` = TYPE|METHOD|NormalizePath|SHA256(payload); `NormalizePath()` заменяет UUID/числа на плейсхолдеры; `HashPayload()`.
> - Подтверждение: [triage.go](internal/triage/triage.go) — `Confirm()` — N реплеев, подтверждение при ≥50% воспроизведений.
> - Минимизация сессий: [triage.go](internal/triage/triage.go) — `MinimizeSession()` — бинарный поиск удаления Exchange.
> - Минимизация JSON body: [triage.go](internal/triage/triage.go) — `MinimizeJSONBody()` — delta-debug JSON-полей (рекурсия до depth 5).
> - Применение в воркере: [worker.go](internal/engine/worker.go) — `handleHit()` — дедупликация → создание Finding → артефакт → confirm → minimize.
> - Интерфейс реплеера для triage: [replayer_iface.go](internal/triage/replayer_iface.go) — `SessionReplayer` interface (абстракция реплея для `Confirm()` / `MinimizeSession()` / `MinimizeJSONBody()`).

### 9.5. Артефакт воспроизведения

Артефакт возвращается через Control API и содержит:

- критерий сбоя `failure_criterion`,

- сессию `RecordingSession` (возможна минимизированная версия).

> 📎 **Реализация (9.5):**
> - [model.go](internal/model/model.go) — `ArtifactPayload`: `FindingID`, `CampaignID`, `Target`, `FailureCriterion` (Type + TimeoutMs), `Session` (RecordingSession), `MutationSeed`, `MutationOps`.
> - Запись артефакта: [worker.go](internal/engine/worker.go) — `writeArtifact()` сериализует в JSON, сохраняет на диск.
> - Чтение через API: [findings.go](internal/api/findings.go) — `getFindingArtifact()`.

### 9.6. Минимальные требования к алгоритмам мутаций

Цель мутаций — генерация новых тестов из seed-сессий путём контролируемых изменений HTTP-запросов и их последовательностей. Мутации должны уметь порождать как синтаксически корректные, так и намеренно пограничные/некорректные входы, при этом не допуская сбоев самой FFUZZ.

#### 9.6.1. Общие требования

1. Каждая мутация должна быть воспроизводимой: для каждого теста сохраняются идентификатор seed, список применённых операторов и seed ГПСЧ.

2. Интенсивность мутаций задаётся параметром кампании `mutations.intensity ∈ [0..1]`.

3. Должны быть ограничения по размерам: максимальная длина URL, заголовков, тела; при превышении — обрезка либо отказ от кандидата.

4. Должны включаться/выключаться классы мутаций: path/query, headers, json body, params, sequence.

> 📎 **Реализация (9.6.1):**
> - Воспроизводимость: [mutate.go](internal/mutate/mutate.go) — `MutationResult{Operators []string, Seed int64}`; [worker.go](internal/engine/worker.go) — seeded RNG `rand.New(rand.NewSource(task.MutationSeed))`; артефакт сохраняет `mutation_seed` + `mutation_ops`.
> - Intensity: [mutate.go](internal/mutate/mutate.go) — `Config.Intensity float64`; [mutate.go](internal/mutate/mutate.go) — `rng.Float64() < intensity` для каждого оператора.
> - Лимиты размеров: [mutate.go](internal/mutate/mutate.go) — `enforceSizeLimits()`: MaxURLLen (default 8192), MaxHdrLen (8192), MaxBodyLen (1MB); обрезка path+query, headers, body.
> - Классы вкл/выкл: [mutate.go](internal/mutate/mutate.go) — `Config{PathQuery, Headers, JSONBody, Params, Sequence bool}`; [engine.go](internal/engine/engine.go).

#### 9.6.2. Базовые байтовые/блочные примитивы

Обязательные операторы:

1. Bit/byte flip: инверсия одного бита/байта.

2. Arithmetic: инкремент/декремент числовых подстрок/байтовых значений в малом дельта-диапазоне.

3. Interesting values: замена на заранее заданные пограничные значения (0, 1, -1, min/max для типовых разрядностей, степени двойки и ±1 вокруг них).

4. Block operations: вставка/удаление/дублирование/замена блока байт.

5. Splicing: склейка частей двух seed-входов одного типа (например, двух query-строк или двух JSON-тел).

> 📎 **Реализация (9.6.2):**
> - BitFlip: [primitive.go](internal/mutate/primitive.go).
> - ByteFlip: [primitive.go](internal/mutate/primitive.go).
> - ArithmeticAdd: [primitive.go](internal/mutate/primitive.go) — wrapping add delta.
> - InterestingReplace: [primitive.go](internal/mutate/primitive.go) — boundary values (0x00, 0xFF, 0x7F, 0x80, 16/32-bit boundaries).
> - BlockOperation: [primitive.go](internal/mutate/primitive.go) — Insert/Delete/Duplicate/Replace (1-32 bytes).
> - Splice: [primitive.go](internal/mutate/primitive.go) — склейка двух byte slices.

#### 9.6.3. Мутации URI: path и query

При мутациях URI необходимо учитывать зарезервированные символы и percent-encoding. ([IETF Datatracker](https://datatracker.ietf.org/doc/html/rfc3986?utm_source=chatgpt.com "RFC 3986 - Uniform Resource Identifier (URI): Generic ..."))

Обязательные операторы:

1. Вставка/удаление/дублирование сегментов path, изменение слешей, добавление пустых сегментов.

2. Мутации query-параметров: добавление параметров, дублирование ключей, перестановка порядка, пустые значения, очень длинные значения.

3. Инъекция/замена на reserved characters `:/?#[]@!$&'()*+,;=` в path/query в вариантах percent-encoding и «как есть».

4. Генерация невалидных/пограничных percent-encoding последовательностей (неполные, смешанный регистр, избыточное кодирование).

> 📎 **Реализация (9.6.3):**
> - Path segments: [uri.go](internal/mutate/uri.go) — insert/delete/duplicate/empty segment.
> - Query params: [uri.go](internal/mutate/uri.go) — add/duplicate key/empty value/long value (256-768 chars).
> - Reserved chars: [uri.go](internal/mutate/uri.go) — инъекция `:/?#[]@!$&'()*+,;=`.
> - Invalid percent-encoding: [uri.go](internal/mutate/uri.go) — `%`, `%0`, `%ZZ`, `%0G`, `%%41`, `%2541`.
> - Slash manipulation: [uri.go](internal/mutate/uri.go) — trailing slash, double slashes, dot segments, encoded slashes.
> - Long value: [uri.go](internal/mutate/uri.go) — 4096-8192+ chars.

#### 9.6.4. Мутации HTTP-заголовков

Обязательные операторы:

1. Добавление/удаление заголовков, дублирование одного заголовка (несколько значений), перестановка порядка.

2. Мутации значений: очень длинные строки, непечатные/неожиданные символы, необычные пробельные последовательности, изменение регистра.

3. Конфликтующие сочетания (как тест-режим): например, одновременно `Content-Length` и `Transfer-Encoding` (допускается отправка без попытки «починить» запрос).

4. Словарь значений для типовых заголовков и возможность добавлять пользовательский словарь.

> 📎 **Реализация (9.6.4):**
> - Add/remove/duplicate: [header.go](internal/mutate/header.go).
> - Long values: [header.go](internal/mutate/header.go) — MaxHdrLen/2..MaxHdrLen.
> - Conflicting: [header.go](internal/mutate/header.go) — Content-Length + Transfer-Encoding и др.
> - Dictionary: [header.go](internal/mutate/header.go) — built-in dict (Content-Type, Authorization, X-Forwarded-For, Cookie и др.) + merge с user dict.

#### 9.6.5. Мутации тела: JSON-aware

Если `Content-Type` указывает JSON, движок должен применять структурные мутации на уровне дерева разбора; если парсинг невозможен — деградировать к байтовым примитивам.

Обязательные структурные операторы:

1. Замена типа значения (string → number, object → array, true/false/null и т. п.).

2. Мутации объектов: удаление/добавление ключей, дублирование ключей, подмена ключей на длинные/пустые/необычные.

3. Мутации массивов: изменение длины, дублирование элементов, вставка элементов разных типов.

4. Пограничные числа и строки: генерация значений на границах диапазонов и строк характерных длин.

5. Стресс вложенности: увеличение глубины вложенности до конфигурируемого предела и обратное упрощение (используется также при минимизации).

> 📎 **Реализация (9.6.5):**
> - Детекция JSON: [json.go](internal/mutate/json.go) — проверка Content-Type; fallback к PrimitiveMutator.
> - Type substitution: [json.go](internal/mutate/json.go) — nil/true/false/float/string/array/object.
> - Object key mutations: [json.go](internal/mutate/json.go) — add/remove/duplicate/rename.
> - Array mutations: [json.go](internal/mutate/json.go) — duplicate/insert mixed type/remove/empty.
> - Boundary values: [json.go](internal/mutate/json.go) — 0, -0, MaxFloat64, MaxInt32/64, NaN, Inf, 1e308.
> - Depth stress: [json.go](internal/mutate/json.go) — 20-100 уровней вложенности.
> - String mutations: [json.go](internal/mutate/json.go) — empty, long (1024/65536), binary, XSS, SQLi, JNDI, template injection, path traversal, CRLF, unicode.

#### 9.6.6. Мутации последовательностей (stateful)

Для seed-сессии (упорядоченного списка Exchange) обязательны операции:

1. Drop: удаление одного Exchange (кроме обязательных setup-шагов, если такие введены).

2. Duplicate: дублирование Exchange.

3. Swap: ограниченная перестановка соседних Exchange.

4. Per-step mutation: применение мутаций к одному выбранному Exchange внутри последовательности.

> 📎 **Реализация (9.6.6):**
> - Drop: [sequence.go](internal/mutate/sequence.go) — удаление (никогда не удаляет index 0).
> - Duplicate: [sequence.go](internal/mutate/sequence.go).
> - Swap: [sequence.go](internal/mutate/sequence.go) — перестановка соседних.
> - Per-step: [sequence.go](internal/mutate/sequence.go) — primitive mutation к одному случайному Exchange.

#### 9.6.7. Мутации параметров (query/form)

Отдельный класс мутаций для инъекции фаззинговых значений в параметры запроса.

Обязательные операторы:

1. Инъекция fuzz-строк (XSS, SQLi, JNDI, template injection, path traversal, CRLF, unicode, boundary strings) в значения query-параметров (GET).

2. Инъекция fuzz-строк в значения form-encoded body параметров (POST application/x-www-form-urlencoded).

3. Если существующие параметры отсутствуют — добавление нового query-параметра с fuzz-значением.

> 📎 **Реализация (9.6.7):**
> - [param.go](internal/mutate/param.go) — `ParamMutator`: парсинг query и form body, выбор случайной поверхности атаки (query или form), инъекция из `fuzzStrings`.
> - Fuzz-словарь: [mutate.go](internal/mutate/mutate.go) — `fuzzStrings` (empty, long strings, XSS, SQLi, JNDI, template injection, path traversal, CRLF, unicode).

---

## 10. Конфигурирование

FFUZZ должен поддерживать конфигурирование через:

- переменные окружения;

- флаги командной строки.

Поддержка конфигурационного файла допускается опционально (если реализована — она не должна противоречить env/flags).

Минимальный набор параметров:

- FFUUZZ_API_ADDRESS / `-a`

- FFUUZZ_PROXY_ADDRESS / `-p`

- FFUUZZ_DATABASE_URI / `-d`

- FFUUZZ_ARTIFACT_DIR / `-o`

- FFUUZZ_REQ_TIMEOUT

- FFUUZZ_SHUTDOWN_TIMEOUT

- FFUUZZ_WORKERS

- FFUUZZ_RPS

> 📎 **Реализация (10):**
> - Env vars + CLI flags: [config.go](internal/config/config.go) — `Load()` читает `os.Getenv("FFUUZZ_*")`, CLI-флаги обрабатываются в пакете `internal/cli` ([serve.go](internal/cli/serve.go), [proxy.go](internal/cli/proxy.go), [record.go](internal/cli/record.go)).
> - `FFUUZZ_API_ADDRESS` / `-a`: [config.go](internal/config/config.go).
> - `FFUUZZ_PROXY_ADDRESS` / `-p`: [config.go](internal/config/config.go).
> - `FFUUZZ_DATABASE_URI` / `-d`: [config.go](internal/config/config.go).
> - `FFUUZZ_ARTIFACT_DIR` / `-o`: [config.go](internal/config/config.go).
> - `FFUUZZ_REQ_TIMEOUT`: [config.go](internal/config/config.go).
> - `FFUUZZ_SHUTDOWN_TIMEOUT`: [config.go](internal/config/config.go).
> - `FFUUZZ_WORKERS`: [config.go](internal/config/config.go).
> - `FFUUZZ_RPS`: [config.go](internal/config/config.go).
> - Дополнительно: `-cert-cache-size`, `-cert-memory-only`, `-cert-dir`, `-tls-no-tickets`, `-max-body` ([config.go](internal/config/config.go)).

---

## 11. Логирование и метрики

Логи:

- структурированные;

- обязательные поля: request_id, campaign_id (если применимо), recording_id (id сессии).

Метрики (/metrics), минимум:

- `ffuuzz_tests_total` (counter)

- `ffuuzz_findings_total{type=...}` (counter)

- гистограмма latency

- размер корпуса

- метрики LRU кэша сертификатов (hits/misses/evictions)

- ошибки CONNECT/hijack

- метрики endpoint resolver (collapses/merges)

Endpoint `/metrics` должен отдавать метрики в текстовом формате Prometheus exposition 0.0.4; Content-Type: `text/plain; version=0.0.4`. ([prometheus.io](https://prometheus.io/docs/specs/om/open_metrics_spec/?utm_source=chatgpt.com "OpenMetrics 1.0"))

> 📎 **Реализация (11):**
> - Структурированные логи: [logging.go](internal/logging/logging.go) — zerolog JSON, `New()`, `WithRequestID()`, `WithCampaignID()`, `WithRecordingID()`.
> - Метрики: [metrics.go](internal/metrics/metrics.go):
>   - `ffuuzz_tests_total` (Counter);
>   - `ffuuzz_findings_total{type}` (CounterVec);
>   - `ffuuzz_request_duration_seconds` (Histogram);
>   - `ffuuzz_corpus_size` (Gauge);
>   - `ffuuzz_cert_cache_hits/misses/evictions_total` (Counters);
>   - `ffuuzz_connect_errors_total{error_class}` (CounterVec);
>   - `ffuuzz_cert_errors_total` (Counter);
>   - `ffuuzz_endpoint_collapses_total` (Counter);
>   - `ffuuzz_endpoint_merges_total` (Counter).
> - Endpoint `/metrics`: [server.go](internal/api/server.go) — `promhttp.HandlerFor()`.

---

## 12. Сводное HTTP API (Control API)

FFUZZ должен предоставлять следующие хендлеры.

> 📎 **Реализация (12):** Все маршруты определены в [server.go](internal/api/server.go) — Gin router, middleware `requestIDMiddleware`, `loggingMiddleware`.

Записи/корпус:

- POST /api/v1/recordings/import — импорт сессий в корпус;

- GET /api/v1/recordings — список сессий;

- GET /api/v1/recordings/tree — иерархическое дерево записей по origin и path;

- GET /api/v1/recordings/{id} — получить сессию;

- GET /api/v1/recordings/export — экспорт записей (с фильтром по host/path_prefix);

- DELETE /api/v1/recordings/by-prefix — массовое удаление записей по origin + path prefix;

- DELETE /api/v1/recordings/{id} — удалить сессию.

Кампании:

- POST /api/v1/campaigns — создать кампанию;

- POST /api/v1/campaigns/{id}/start — запустить кампанию;

- POST /api/v1/campaigns/{id}/stop — остановить кампанию;

- GET /api/v1/campaigns — список кампаний;

- GET /api/v1/campaigns/{id} — детали кампании;

- GET /api/v1/campaigns/{id}/stats — статистика кампании;

- GET /api/v1/campaigns/{id}/findings — находки кампании;

- GET /api/v1/campaigns/{id}/config — конфигурация кампании;

- GET /api/v1/campaigns/{id}/stream — SSE-стрим статистики в реальном времени;

- POST /api/v1/campaigns/{id}/recordings — добавление записей в кампанию по фильтру origin + path prefix.

Находки:

- GET /api/v1/findings — листинг находок (кросс-кампанийный, с опциональным фильтром `campaign_id`);

- GET /api/v1/findings/{id} — детали находки;

- GET /api/v1/findings/{id}/artifact — артефакт воспроизведения;

- POST /api/v1/findings/{id}/reproduce — принудительное воспроизведение (асинхронно).

Служебное:

- GET /healthz — healthcheck;

- GET /metrics — метрики Prometheus.

---

## 13. Детальные спецификации HTTP API

### Общие договорённости

- Base path: `/api/v1`.

- Все тела запросов/ответов (кроме `/metrics`) — `application/json`.

- Все ответы содержат `X-Request-ID`.

- Пустые ответы (204) — без тела.

- Формат ошибок — раздел 8.3.

> 📎 **Реализация (13 общее):**
> - Base path `/api/v1`: [server.go](internal/api/server.go) — `router.Group("/api/v1")`.
> - `X-Request-ID` во всех ответах: [server.go](internal/api/server.go) — `requestIDMiddleware`.
> - 204 без тела: [recordings.go](internal/api/recordings.go), [campaigns.go](internal/api/campaigns.go), [findings.go](internal/api/findings.go).
> - Формат ошибок: [server.go](internal/api/server.go) — `errorResponse()` / `internalError()`.

### 13.1. Импорт записей

Хендлер: POST /api/v1/recordings/import.

> 📎 **Реализация:** [recordings.go](internal/api/recordings.go) — `importRecordings()`: Content-Type validation, 10MB body limit, duplicate ID detection, idempotent upsert via `RecordingStore.Upsert()`, инкремент `metrics.CorpusSize`.

Назначение: импортировать одну или несколько `RecordingSession`. Идемпотентность по `sessions[i].id`: если сессия уже существует, импорт считается skipped.

Формат запроса:

```http
POST /api/v1/recordings/import HTTP/1.1
Content-Type: application/json
...

{
  "sessions": [ <RecordingSession>, ... ]
}
```

Возможные коды ответа:

- 201 — импорт завершён (в том числе частично);

- 400 — неверный формат запроса;

- 409 — дубли `sessions[].id` внутри запроса;

- 413 — слишком большой запрос;

- 415 — неподдерживаемый Content-Type;

- 500 — внутренняя ошибка сервера.

Формат ответа:

```http
201 Created HTTP/1.1
Content-Type: application/json
X-Request-ID: ...
...

{
  "imported": 2,
  "skipped": 1,
  "failed": 0,
  "total": 3,
  "session_ids": ["...","..."],
  "skipped_session_ids": ["..."],
  "errors": [
    "session <id>: <message>",
    "..."
  ]
}
```

Поле `errors` возвращается только если `failed > 0`.

### 13.2. Список сессий

Хендлер: GET /api/v1/recordings.

> 📎 **Реализация:** [recordings.go](internal/api/recordings.go) — `listRecordings()`: limit (default 50), offset, host filter, path_prefix filter; 204 если пусто.

Назначение: листинг сессий (без `entries`).

Query params:

- limit (int, default 50)

- offset (int, default 0)

- host (string, опционально — фильтр по хосту)

- path_prefix (string, опционально — фильтр по префиксу пути)

Формат запроса:

```http
GET /api/v1/recordings?limit=50&offset=0&host=example.local&path_prefix=/api/users HTTP/1.1
Content-Length: 0
```

Возможные коды ответа:

- 200 — успешная обработка;

- 204 — нет данных для ответа;

- 400 — неверные query params;

- 500 — внутренняя ошибка сервера.

Формат ответа:

```http
200 OK HTTP/1.1
Content-Type: application/json
X-Request-ID: ...
...

[
  {
    "id": "uuid",
    "schema_version": 1,
    "created_at": "RFC3339",
    "target": { "scheme": "https", "host": "example.local", "port": 443, "path": "/api/users/{_}" },
    "entry_count": 12
  }
]
```

### 13.2a. Дерево записей

Хендлер: GET /api/v1/recordings/tree.

> 📎 **Реализация:** [recordings.go](internal/api/recordings.go) — `getRecordingsTree()` / `buildTree()`: агрегация записей по origin (scheme://host:port), построение иерархии path-сегментов через trie.

Назначение: получить иерархическое дерево записей, сгруппированных по origin и path-сегментам. Используется Web UI для навигации по записанным endpoint'ам.

Формат запроса:

```http
GET /api/v1/recordings/tree HTTP/1.1
Content-Length: 0
```

Возможные коды ответа:

- 200 — успешная обработка;

- 500 — внутренняя ошибка сервера.

Формат ответа:

```http
200 OK HTTP/1.1
Content-Type: application/json
X-Request-ID: ...
...

[
  {
    "origin": "https://example.local:443",
    "scheme": "https",
    "host": "example.local",
    "port": 443,
    "recording_count": 25,
    "paths": [
      {
        "segment": "api",
        "full_path": "/api",
        "recording_count": 0,
        "children": [
          {
            "segment": "users",
            "full_path": "/api/users/{_}",
            "recording_count": 15,
            "children": []
          }
        ]
      }
    ]
  }
]
```

### 13.2b. Экспорт записей

Хендлер: GET /api/v1/recordings/export.

> 📎 **Реализация:** [recordings.go](internal/api/recordings.go) — `exportRecordings()`: host/path_prefix фильтры; возвращает все записи (включая entries) с заголовком `Content-Disposition: attachment`.

Назначение: экспортировать записи в формате JSON для переноса между инстансами FFUZZ или резервного копирования.

Query params:

- host (string, опционально — фильтр по хосту)

- path_prefix (string, опционально — фильтр по префиксу пути)

Формат запроса:

```http
GET /api/v1/recordings/export?host=example.local&path_prefix=/api/users HTTP/1.1
Content-Length: 0
```

Возможные коды ответа:

- 200 — успешная обработка;

- 500 — внутренняя ошибка сервера.

Формат ответа:

```http
200 OK HTTP/1.1
Content-Type: application/json
Content-Disposition: attachment; filename="recordings-export.json"
X-Request-ID: ...
...

{
  "sessions": [ <RecordingSession>, ... ]
}
```

### 13.3. Получение сессии по id

Хендлер: GET /api/v1/recordings/{id}.

> 📎 **Реализация:** [recordings.go](internal/api/recordings.go) — `getRecording()`: include_entries (default false), max_body_bytes (default 0).

Назначение: получить `RecordingSession`; по умолчанию без `entries`.

Query params:

- include_entries (true/false, default false)

- max_body_bytes (int, default 0 = unlimited, max 1048576)

Формат запроса:

```http
GET /api/v1/recordings/{id}?include_entries=true&max_body_bytes=65536 HTTP/1.1
Content-Length: 0
```

Возможные коды ответа:

- 200 — успешная обработка;

- 404 — запись не найдена;

- 400 — неверные query params;

- 500 — внутренняя ошибка сервера.

Формат ответа (include_entries=0):

```http
200 OK HTTP/1.1
Content-Type: application/json
X-Request-ID: ...
...

{
  "schema_version": 1,
  "id": "uuid",
  "created_at": "RFC3339",
  "target": { "scheme": "https", "host": "example.local", "port": 443, "path": "/api/users/{_}" },
  "entry_count": 12
}
```

Формат ответа (include_entries=1): полный `RecordingSession v1` (раздел 8.1/8.2).

### 13.4. Удаление сессии

Хендлер: DELETE /api/v1/recordings/{id}.

> 📎 **Реализация:** [recordings.go](internal/api/recordings.go) — `deleteRecording()`: проверка `IsUsedByActiveCampaign()` → 409; декремент `metrics.CorpusSize`.

Назначение: удалить сессию из корпуса. Если сессия используется в кампании со статусом RUNNING или STARTING — запретить.

Формат запроса:

```http
DELETE /api/v1/recordings/{id} HTTP/1.1
Content-Length: 0
```

Возможные коды ответа:

- 204 — запись удалена;

- 404 — запись не найдена;

- 409 — запись привязана к активной кампании;

- 500 — внутренняя ошибка сервера.

### 13.4a. Массовое удаление записей по prefix

Хендлер: DELETE /api/v1/recordings/by-prefix.

> 📎 **Реализация:** [recordings.go](internal/api/recordings.go) — `deleteRecordingsByPrefix()`: обязательные параметры scheme, host, port; опциональный path_prefix; проверка на использование активными кампаниями → 409; декремент `metrics.CorpusSize` для каждой удалённой записи.

Назначение: массовое удаление записей, соответствующих указанному origin (scheme + host + port) и опциональному префиксу пути.

Query params:

- scheme (string, обязательный)

- host (string, обязательный)

- port (int, обязательный)

- path_prefix (string, опционально)

Формат запроса:

```http
DELETE /api/v1/recordings/by-prefix?scheme=https&host=example.local&port=443&path_prefix=/api/users HTTP/1.1
Content-Length: 0
```

Возможные коды ответа:

- 200 — удаление выполнено;

- 400 — отсутствуют обязательные параметры;

- 409 — часть записей используется активными кампаниями;

- 500 — внутренняя ошибка сервера.

Формат ответа:

```http
200 OK HTTP/1.1
Content-Type: application/json
X-Request-ID: ...
...

{ "deleted": 5 }
```

### 13.5. Создание кампании

Хендлер: POST /api/v1/campaigns.

> 📎 **Реализация:** [campaigns.go](internal/api/campaigns.go) — `createCampaign()`: валидация config, проверка recording_ids, `CampaignStore.Create()` (транзакция).

Назначение: создать кампанию фаззинга на основе `recording_ids`.

Формат запроса:

```http
POST /api/v1/campaigns HTTP/1.1
Content-Type: application/json
...

{
  "name": "govwa-login-fuzz",
  "recording_ids": ["uuid", "..."],
  "config": {
    "target": { "base_url": "http://localhost:8080" },
    "limits": {
      "workers": 8,
      "rps": 50,
      "max_tests": 20000,
      "duration_sec": 3600,
      "req_timeout_ms": 3000
    },
    "mutations": {
      "path_query": true,
      "headers": true,
      "json_body": true,
      "params": true,
      "sequence": true,
      "intensity": 0.6
    },
    "anomaly": {
      "detect_5xx": true,
      "latency_multiplier": 3.0,
      "regex_patterns": ["panic", "stack trace", "SQL error"]
    },
    "triage": {
      "confirm_runs": 3,
      "enable_minimization": true
    }
  }
}
```

Поле `anomaly.regex_patterns` опционально; если не задано, детект REGEX_MATCH отключён.

Возможные коды ответа:

- 201 — кампания создана;

- 400 — неверный формат запроса;

- 404 — один из recording_ids не найден;

- 422 — логически некорректные параметры;

- 500 — внутренняя ошибка сервера.

Формат ответа:

```http
201 Created HTTP/1.1
Content-Type: application/json
X-Request-ID: ...
...

{
  "id": "uuid",
  "name": "govwa-login-fuzz",
  "status": "CREATED",
  "created_at": "RFC3339",
  "updated_at": "RFC3339",
  "recording_ids": ["uuid","..."],
  "progress": { "tests_done": 0, "findings_total": 0 }
}
```

### 13.6. Запуск кампании

Хендлер: POST /api/v1/campaigns/{id}/start.

> 📎 **Реализация:** [campaigns.go](internal/api/campaigns.go) — `startCampaign()`: CREATED/STOPPED/FINISHED/FAILED → STARTING; `engine.StartCampaign()`. 202 Accepted.

Назначение: асинхронно запустить кампанию.

Формат запроса:

```http
POST /api/v1/campaigns/{id}/start HTTP/1.1
Content-Type: application/json
...

{ "resume": false }
```

Возможные коды ответа:

- 202 — запрос принят, запуск выполняется;

- 404 — кампания не найдена;

- 409 — кампания уже RUNNING/STARTING;

- 422 — состояние не допускает запуск;

- 500 — внутренняя ошибка сервера.

Формат ответа:

```http
202 Accepted HTTP/1.1
Content-Type: application/json
X-Request-ID: ...
...

{ "id": "uuid", "status": "STARTING", "updated_at": "RFC3339" }
```

### 13.7. Остановка кампании

Хендлер: POST /api/v1/campaigns/{id}/stop.

> 📎 **Реализация:** [campaigns.go](internal/api/campaigns.go) — `stopCampaign()`: RUNNING/STARTING → STOPPING; `engine.StopCampaign()`. 202 Accepted.

Назначение: асинхронно остановить кампанию.

Формат запроса:

```http
POST /api/v1/campaigns/{id}/stop HTTP/1.1
Content-Length: 0
```

Возможные коды ответа:

- 202 — запрос принят, остановка выполняется;

- 404 — кампания не найдена;

- 409 — кампания не RUNNING/STARTING;

- 500 — внутренняя ошибка сервера.

Формат ответа:

```http
202 Accepted HTTP/1.1
Content-Type: application/json
X-Request-ID: ...
...

{ "id": "uuid", "status": "STOPPING", "updated_at": "RFC3339" }
```

### 13.8. Список кампаний

Хендлер: GET /api/v1/campaigns.

> 📎 **Реализация:** [campaigns.go](internal/api/campaigns.go) — `listCampaigns()`: status/limit/offset фильтры; 204 если пусто.

Назначение: листинг кампаний.

Формат запроса:

```http
GET /api/v1/campaigns?status=RUNNING&limit=50&offset=0 HTTP/1.1
Content-Length: 0
```

Возможные коды ответа:

- 200 — успешная обработка;

- 204 — нет данных для ответа;

- 400 — неверные query params;

- 500 — внутренняя ошибка сервера.

Формат ответа:

```http
200 OK HTTP/1.1
Content-Type: application/json
X-Request-ID: ...
...

[
  {
    "id": "uuid",
    "name": "govwa-login-fuzz",
    "status": "RUNNING",
    "created_at": "RFC3339",
    "updated_at": "RFC3339",
    "started_at": "RFC3339",
    "finished_at": null,
    "progress": { "tests_done": 12345, "findings_total": 12 }
  }
]
```

### 13.9. Получение кампании по id

Хендлер: GET /api/v1/campaigns/{id}.

> 📎 **Реализация:** [campaigns.go](internal/api/campaigns.go) — `getCampaign()`: `CampaignStore.GetByID()` с recording_ids.

Назначение: получить полную конфигурацию кампании и состояние.

Формат запроса:

```http
GET /api/v1/campaigns/{id} HTTP/1.1
Content-Length: 0
```

Возможные коды ответа:

- 200 — успешная обработка;

- 404 — кампания не найдена;

- 500 — внутренняя ошибка сервера.

Формат ответа:

```http
200 OK HTTP/1.1
Content-Type: application/json
X-Request-ID: ...
...

{
  "id": "uuid",
  "name": "string",
  "status": "RUNNING",
  "created_at": "RFC3339",
  "updated_at": "RFC3339",
  "started_at": "RFC3339",
  "finished_at": null,
  "recording_ids": ["uuid"],
  "progress": { "tests_done": 12345, "findings_total": 12 }
}
```

### 13.10. Статистика кампании

Хендлер: GET /api/v1/campaigns/{id}/stats.

> 📎 **Реализация:** [campaigns.go](internal/api/campaigns.go) — `getCampaignStats()` / `buildCampaignStats()`: tests_total, tests_per_sec, counts by finding type, seeds info.

Назначение: агрегированная статистика по кампании.

Формат запроса:

```http
GET /api/v1/campaigns/{id}/stats HTTP/1.1
Content-Length: 0
```

Возможные коды ответа:

- 200 — успешная обработка;

- 404 — кампания не найдена;

- 500 — внутренняя ошибка сервера.

Формат ответа:

```http
200 OK HTTP/1.1
Content-Type: application/json
X-Request-ID: ...
...

{
  "campaign_id": "uuid",
  "status": "RUNNING",
  "tests_total": 12345,
  "tests_per_sec": 102.3,
  "timeouts": 12,
  "server_errors": 7,
  "latency_regressions": 3,
  "regex_matches": 0,
  "last_activity_at": "RFC3339",
  "seeds": {
    "sessions_total": 10,
    "sessions_used": 7,
    "exchanges_sent": 25000
  }
}
```

### 13.11. Список находок кампании

Хендлер: GET /api/v1/campaigns/{id}/findings.

> 📎 **Реализация:** [campaigns.go](internal/api/campaigns.go) — `getCampaignFindings()`: type/status/since/limit/offset фильтры; 204 если пусто.

Назначение: листинг находок по кампании.

Query params:

- limit, offset

- type (TIMEOUT|SERVER_ERROR|LATENCY_REGRESSION|REGEX_MATCH)

- status (UNCONFIRMED|CONFIRMED)

- since (RFC3339)

Формат запроса:

```http
GET /api/v1/campaigns/{id}/findings?type=TIMEOUT&limit=50 HTTP/1.1
Content-Length: 0
```

Возможные коды ответа:

- 200 — успешная обработка;

- 204 — нет данных для ответа;

- 404 — кампания не найдена;

- 400 — неверные query params;

- 500 — внутренняя ошибка сервера.

Формат ответа:

```http
200 OK HTTP/1.1
Content-Type: application/json
X-Request-ID: ...
...

[
  {
    "id": "uuid",
    "campaign_id": "uuid",
    "type": "TIMEOUT",
    "status": "CONFIRMED",
    "signature": "TIMEOUT|POST|/login|hash:...",
    "created_at": "RFC3339",
    "method": "POST",
    "endpoint": "/login",
    "seed_recording_id": "uuid",
    "artifact_id": "uuid"
  }
]
```

### 13.11a. Конфигурация кампании

Хендлер: GET /api/v1/campaigns/{id}/config.

> 📎 **Реализация:** [campaign_config.go](internal/api/campaign_config.go) — `getCampaignConfig()`: возвращает `Campaign.Config` (CampaignConfig struct).

Назначение: вернуть JSON-объект конфигурации кампании (`CampaignConfig`).

Формат запроса:

```http
GET /api/v1/campaigns/{id}/config HTTP/1.1
Content-Length: 0
```

Возможные коды ответа:

- 200 — успешная обработка;

- 404 — кампания не найдена;

- 500 — внутренняя ошибка сервера.

Формат ответа: JSON-объект `CampaignConfig` (аналогичен полю `config` при создании кампании, см. 13.5).

### 13.11b. SSE-стрим статистики кампании

Хендлер: GET /api/v1/campaigns/{id}/stream.

> 📎 **Реализация:** [sse.go](internal/api/sse.go) — `streamCampaignStats()` / `sendStatsEvent()`: SSE headers (text/event-stream, no-cache, keep-alive, X-Accel-Buffering: no), отправка каждые 2 секунды, `event: done` при терминальных статусах.

Назначение: Server-Sent Events (SSE) эндпоинт для потоковой отправки статистики кампании в реальном времени.

Формат запроса:

```http
GET /api/v1/campaigns/{id}/stream HTTP/1.1
Accept: text/event-stream
```

Возможные коды ответа:

- 200 — поток SSE открыт;

- 404 — кампания не найдена;

- 500 — внутренняя ошибка сервера.

Поведение:

- Заголовки ответа: `Content-Type: text/event-stream`, `Cache-Control: no-cache`, `Connection: keep-alive`, `X-Accel-Buffering: no`.

- Каждые 2 секунды сервер отправляет `data: <JSON CampaignStats>\n\n`.

- При достижении терминального статуса (FINISHED, FAILED, STOPPED) отправляется `event: done\ndata: <JSON>\n\n` и поток закрывается.

### 13.11c. Добавление записей в кампанию

Хендлер: POST /api/v1/campaigns/{id}/recordings.

> 📎 **Реализация:** [campaigns.go](internal/api/campaigns.go) — `addRecordingsToCampaign()`: проверка состояния кампании, `CampaignStore.AddRecordingsByFilter()` по scheme/host/port/path_prefix.

Назначение: массовое добавление записей в существующую кампанию по фильтру origin (scheme + host + port) и опциональному префиксу пути. Кампания не должна быть в состоянии RUNNING или STARTING.

Формат запроса:

```http
POST /api/v1/campaigns/{id}/recordings HTTP/1.1
Content-Type: application/json
...

{
  "scheme": "https",
  "host": "example.local",
  "port": 443,
  "path_prefix": "/api/users"
}
```

Возможные коды ответа:

- 200 — записи добавлены;

- 400 — неверный формат запроса;

- 404 — кампания не найдена;

- 409 — кампания в состоянии RUNNING/STARTING;

- 500 — внутренняя ошибка сервера.

Формат ответа:

```http
200 OK HTTP/1.1
Content-Type: application/json
X-Request-ID: ...
...

{ "added": 5 }
```

### 13.11d. Листинг находок (кросс-кампанийный)

Хендлер: GET /api/v1/findings.

> 📎 **Реализация:** [findings.go](internal/api/findings.go) — `listFindings()`: campaign_id/type/status/since/limit/offset фильтры; `FindingStore.ListAll()` с dynamic query builder.

Назначение: листинг находок по всем кампаниям с опциональными фильтрами.

Query params:

- limit, offset

- campaign_id (опционально — фильтр по кампании)

- type (TIMEOUT|SERVER_ERROR|LATENCY_REGRESSION|REGEX_MATCH)

- status (UNCONFIRMED|CONFIRMED)

- since (RFC3339)

Формат запроса:

```http
GET /api/v1/findings?campaign_id=uuid&type=TIMEOUT&limit=50 HTTP/1.1
Content-Length: 0
```

Возможные коды ответа:

- 200 — успешная обработка;

- 204 — нет данных для ответа;

- 400 — неверные query params;

- 500 — внутренняя ошибка сервера.

Формат ответа: массив объектов Finding (аналогично 13.11).

### 13.12. Получение finding по id

Хендлер: GET /api/v1/findings/{id}.

> 📎 **Реализация:** [findings.go](internal/api/findings.go) — `getFinding()`: `FindingStore.GetByID()` с artifact_id.

Назначение: получить детальную карточку находки.

Формат запроса:

```http
GET /api/v1/findings/{id} HTTP/1.1
Content-Length: 0
```

Возможные коды ответа:

- 200 — успешная обработка;

- 404 — не найдено;

- 500 — внутренняя ошибка сервера.

Формат ответа:

```http
200 OK HTTP/1.1
Content-Type: application/json
X-Request-ID: ...
...

{
  "id": "uuid",
  "campaign_id": "uuid",
  "type": "TIMEOUT",
  "status": "CONFIRMED",
  "signature": "TIMEOUT|POST|/login|hash:...",
  "created_at": "RFC3339",
  "confirmed_at": "RFC3339",
  "method": "POST",
  "endpoint": "/login",
  "details": {
    "baseline_ms": 120,
    "observed_ms": 3100,
    "timeout_ms": 3000,
    "http_status": 0
  },
  "seed_recording_id": "uuid",
  "artifact_id": "uuid",
  "minimized": true
}
```

### 13.13. Скачивание артефакта воспроизведения

Хендлер: GET /api/v1/findings/{id}/artifact.

> 📎 **Реализация:** [findings.go](internal/api/findings.go) — `getFindingArtifact()`: `ArtifactStore.GetByFindingID()` → чтение файла с диска → Content-Type: application/json.

Назначение: вернуть JSON-артефакт воспроизведения: `failure_criterion` + `session` (RecordingSession).

Формат запроса:

```http
GET /api/v1/findings/{id}/artifact HTTP/1.1
Content-Length: 0
```

Возможные коды ответа:

- 200 — успешная обработка;

- 404 — finding или артефакт не найден;

- 500 — внутренняя ошибка сервера.

Формат ответа:

```http
200 OK HTTP/1.1
Content-Type: application/json
X-Request-ID: ...
...

{
  "finding_id": "uuid",
  "campaign_id": "uuid",
  "target": { "base_url": "http://localhost:8080" },
  "failure_criterion": { "type": "TIMEOUT", "timeout_ms": 3000 },
  "session": { <RecordingSession> }
}
```

### 13.14. Принудительное воспроизведение finding

Хендлер: POST /api/v1/findings/{id}/reproduce.

> 📎 **Реализация:** [findings.go](internal/api/findings.go) — `reproduceFinding()`: runs 1-20 (default 3), 409 если ENQUEUED/RUNNING; `FindingStore.UpdateReproduceStatus(ENQUEUED)`. Фоновый воркер: [reproduce.go](internal/engine/reproduce.go) — polling каждые 5s, `ClaimNextReproduceJob()` (SKIP LOCKED), N реплеев, majority threshold.

Назначение: инициировать повторное воспроизведение finding с текущим артефактом (асинхронно).

Формат запроса:

```http
POST /api/v1/findings/{id}/reproduce HTTP/1.1
Content-Type: application/json
...

{ "runs": 3 }
```

Возможные коды ответа:

- 202 — запрос принят;

- 404 — finding не найден;

- 409 — воспроизведение уже выполняется;

- 422 — runs вне диапазона (1..20);

- 500 — внутренняя ошибка сервера.

Формат ответа:

```http
202 Accepted HTTP/1.1
Content-Type: application/json
X-Request-ID: ...
...

{
  "finding_id": "uuid",
  "reproduce_status": "ENQUEUED",
  "runs": 3,
  "enqueued_at": "RFC3339"
}
```

### 13.15. Healthcheck

Хендлер: GET /healthz.

> 📎 **Реализация:** [server.go](internal/api/server.go) — `healthz()`: DB ping check, returns `status: ok/degraded`, `db: ok/<error>`, `version: 0.1.0`, `time: RFC3339 UTC`. 503 если DB недоступна.

Назначение: health endpoint (включая проверку доступности БД).

Формат запроса:

```http
GET /healthz HTTP/1.1
Content-Length: 0
```

Возможные коды ответа:

- 200 — сервис работоспособен;

- 503 — сервис не готов (например, БД недоступна);

- 500 — внутренняя ошибка сервера.

Формат ответа:

```http
200 OK HTTP/1.1
Content-Type: application/json
X-Request-ID: ...
...

{
  "status": "ok",
  "db": "ok",
  "version": "0.1.0",
  "time": "RFC3339"
}
```

### 13.16. Метрики Prometheus

Хендлер: GET /metrics.

> 📎 **Реализация:** [server.go](internal/api/server.go) — `metricsHandler()`: `promhttp.HandlerFor()` с Prometheus registry.

Назначение: отдать метрики в формате Prometheus exposition 0.0.4.

Формат запроса:

```http
GET /metrics HTTP/1.1
Content-Length: 0
```

Возможные коды ответа:

- 200 — успешная обработка запроса;

- 500 — внутренняя ошибка сервера.

Формат ответа:

```http
200 OK HTTP/1.1
Content-Type: text/plain; version=0.0.4

# HELP ffuuzz_tests_total Total number of fuzz tests executed.
# TYPE ffuuzz_tests_total counter
ffuuzz_tests_total 12345
```

---

## 14. Приёмочные критерии (минимум)

1. FFUZZ поднимается локально (docker-compose допускается) и корректно завершает работу по SIGINT/SIGTERM в пределах `FFUZZ_SHUTDOWN_TIMEOUT`.

2. Прокси поддерживает HTTP и HTTPS MITM, генерирует сертификаты, использует кэш с LRU-вытеснением и метриками.

3. Рекордер пишет `RecordingSession` и позволяет импортировать сессии через Control API.

4. Кампания создаётся, запускается, останавливается через Control API; доступны статистика и список находок.

5. Движок генерирует мутации на базе записанных сессий и фиксирует минимум одну находку на выбранном SUT.

6. Для находки доступен артефакт воспроизведения; воспроизведение возможно через /reproduce или эквивалентный механизм, предусмотренный реализацией.

7. /metrics экспортирует метрики.

> 📎 **Реализация (14):**
> - П. 1: [serve.go](internal/cli/serve.go) — `runServe()`, graceful shutdown по SIGINT/SIGTERM; docker-compose.yml в корне проекта.
> - П. 2: [mitm.go](internal/mitm/mitm.go), [store.go](internal/store/store.go) — proxy + cert store + LRU cache + metrics.
> - П. 3: [recorder.go](internal/recorder/recorder.go), [recordings.go](internal/api/recordings.go) — запись + импорт.
> - П. 4: [campaigns.go](internal/api/campaigns.go) — CRUD + start/stop; [campaigns.go](internal/api/campaigns.go) — stats; [campaigns.go](internal/api/campaigns.go) — findings list.
> - П. 5: `internal/mutate/` + `internal/engine/` + `internal/anomaly/` — мутации + движок + детект.
> - П. 6: [findings.go](internal/api/findings.go) — artifact; [findings.go](internal/api/findings.go) — reproduce; [reproduce.go](internal/engine/reproduce.go) — воркер.
> - П. 7: [server.go](internal/api/server.go) — /metrics endpoint.

---

## 15. Deliverables

- исходный код FFUZZ (прокси, рекордер, движок, Control API);

- миграции/инициализация БД;

- README с шагами запуска SUT (Docker), запуска FFUZZ, записи, импорта и запуска кампании;

- набор тестов (unit + минимум один интеграционный сценарий).

> 📎 **Реализация (15):**
> - Исходный код: `cmd/ffuuzz/`, `internal/` (20 пакетов: cli, mitm, api, engine, mutate, anomaly, triage, replayer, corpus, recorder, db, store, config, metrics, logging, httputil, diff, report, model, endpoint), `web/`.
> - Миграции: [internal/db/migrations/](internal/db/migrations/) — `001_initial.up.sql`, `002_reproduce_runs.up.sql`, `003_recording_target_path.up.sql`, `004_finding_mutation.up.sql`.
> - README: [README.md](README.md) — содержит инструкции по запуску PostgreSQL (docker-compose), FFUZZ (serve/proxy/record), записи трафика, импорту сессий и запуску кампании через Web UI.
> - Тесты: 28 файлов `*_test.go` в 18 пакетах (anomaly, api, config, corpus, db, diff, endpoint, engine, httputil, logging, metrics, mitm, mutate, recorder, replayer, report, store, triage).
