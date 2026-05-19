# Роадмап развития FFUUZZ в 2026

## Текущее состояние проекта

**FFUUZZ** -- L7-фаззер на базе MITM-прокси + рекордера. ~8,100 LOC Go backend, ~3,700 LOC React/TS frontend, 80.1% тестовое покрытие. Текущие возможности:

- **Мутации**: 6 типов мутаторов (URI, Header, JSON, Param, Primitive, Sequence), 26+ операций
- **Детект аномалий**: 4 детектора (Timeout, 5xx, Latency Regression, Regex Match) -- все на формальных правилах
- **Триаж**: дедупликация по сигнатуре, подтверждение N-replay, минимизация сессий и JSON body (delta-debug)
- **Эндпоинты**: только custom JSON import + MITM proxy recording; нет HAR/Swagger/crawling
- **Протоколы**: только HTTP/HTTPS
- **UI**: React 19 + DaisyUI, базовый dashboard
- **Нет**: MCP-сервера, агентной системы, ML/LLM-интеграции, gRPC

---

## 1. Улучшение и расширение мутаций

### ~~1.1 Улучшение существующих мутаторов~~

~~**Исследование**: анализ coverage-guided fuzzing подходов (AFL, libFuzzer) для адаптации к HTTP-фаззингу -- feedback loop на основе response diversity, а не code coverage.~~

~~**Реализация**:~~

- ~~**Coverage-guided feedback**: расширить воркер для отслеживания "интересности" мутаций (новые status codes, новые error messages, новые response structures); приоритизировать seed'ы, порождающие разнообразные ответы; добавить `interestScore` в [worker.go](internal/engine/worker.go) и feedback в цикл генерации задач [engine.go](internal/engine/engine.go)~~
- ~~**Adaptive intensity**: динамическое управление `intensity` на основе статистики находок -- увеличивать интенсивность для "продуктивных" мутаторов; реализовать в [mutate.go](internal/mutate/mutate.go) как weighted random selection~~
- ~~**Расширение `fuzzStrings`** в [mutate.go](internal/mutate/mutate.go): добавить XXE payloads (`<!DOCTYPE...>`), SSRF payloads (`http://169.254.169.254/...`), command injection payloads (`` `id` ``, `$(whoami)`), prototype pollution payloads (`__proto__`, `constructor`)~~
- ~~**User dictionary as first-class**: расширить `Config.UserDictionary` для поддержки загрузки из файлов и per-endpoint словарей; интеграция с corpus-awareness (извлекать реальные значения из записанного трафика)~~

### 1.2 Новые мутаторы

**Исследование**: изучить подходы grammar-aware fuzzing (Dharma, Domato) для генерации структурно-валидных мутаций.

**Реализация**:

- **XMLMutator**: аналог JSONMutator для XML/SOAP body -- инъекции XXE, entity expansion, namespace manipulation, CDATA injection; определять по `Content-Type: application/xml|text/xml`
- **MultipartMutator**: мутации `multipart/form-data` -- манипуляция boundary, Content-Disposition injection, filename traversal, double extensions (`.php.jpg`), oversized parts
- **GraphQLMutator**: парсинг GraphQL queries -- field injection, depth bombing, alias abuse, introspection queries, batch mutation
- **AuthMutator**: целенаправленные мутации аутентификационных заголовков -- JWT manipulation (algorithm confusion, signature removal, expired tokens, wrong audience), cookie tampering, CSRF token bypass
- **CORSMutator**: мутации Origin/Referer/Access-Control-\* заголовков для тестирования CORS policy
- **EncodingMutator**: систематическое тестирование encoding chains -- double URL-encoding, mixed UTF-8/UTF-16, overlong UTF-8 sequences, null byte injection через разные encodings
- **Grammar-aware string mutator**: использование BNF/PEG грамматик для генерации структурно-валидных, но семантически некорректных значений (корректный JSON c невалидным содержимым, валидный email с injection payload)

### 1.3 Stateful-aware мутации

**Исследование**: анализ подходов RESTler (Microsoft) к inference-based stateful fuzzing -- автоматическое обнаружение producer-consumer зависимостей между endpoints.

**Реализация**:

- **Dependency inference**: анализ записанного трафика для автоматического обнаружения зависимостей (e.g., POST /users возвращает `id`, который используется в GET /users/{id}); расширить [corpus.go](internal/corpus/corpus.go)
- **Semantic sequence mutations**: вместо случайного swap/drop в [sequence.go](internal/mutate/sequence.go) -- осмысленные перестановки с учётом зависимостей (удаление setup-шага, replay cleanup без creation)
- **State-machine exploration**: построение конечного автомата из записанного трафика и систематический обход неисследованных переходов

---

## 2. Расширение детекции аномалий

### 2.1 Новые формальные детекторы

**Реализация** (расширение [detector.go](internal/anomaly/detector.go)):

- **StatusCodeChangeDetector**: детект любого изменения status code относительно baseline (не только 5xx); настраиваемые whitelist/blacklist кодов
- **ResponseSizeDetector**: аномалия если `len(response_body)` отклоняется от baseline на >N% или >K стандартных отклонений; требует расширения `BaselineEntry` в [corpus.go](internal/corpus/corpus.go) для хранения size statistics
- **HeaderChangeDetector**: детект появления/исчезновения security-relevant заголовков (CSP, X-Frame-Options, CORS headers); детект information disclosure через новые заголовки (X-Powered-By, Server version leaks)
- **DiffDetector**: структурное сравнение response body с baseline -- детект появления новых JSON-ключей, изменения типов, появления stack traces; расширить [diff.go](internal/diff/diff.go)
- **ConnectionResetDetector**: детект TCP RST, connection refused, DNS failures -- индикаторы crash/DoS
- **RedirectDetector**: детект open redirect -- анализ Location заголовка на наличие user-controlled redirect targets
- **ReflectionDetector**: поиск отражённого input в response body -- базовый XSS-индикатор без ML

### 2.2 Статистические и ML-детекторы

**Исследование**: изучить применимость Isolation Forest, One-Class SVM, autoencoders для anomaly detection в HTTP-трафике; оценить trade-off inline vs. batch detection.

**Реализация**:

- **Statistical baseline enhancement**: заменить простой P50 в [corpus.go](internal/corpus/corpus.go) на полный профиль (P50, P90, P99, mean, stddev, min, max) для latency, response size, и status code distribution; использовать z-score для anomaly scoring
- **Response clustering**: offline-кластеризация response body (TF-IDF + k-means или DBSCAN) для обнаружения аномальных response-шаблонов (error pages, debug output, information leaks); реализовать как отдельный пакет `internal/ml/`
- **Isolation Forest detector**: обучение на baseline features (latency, size, status, header count, content-type); inference inline per-exchange; использовать Go-native реализацию или cgo binding к lightweight C library
- **Time-series anomaly detection**: обнаружение degradation patterns во времени кампании (постепенный рост latency, увеличение error rate) -- EWMA или Holt-Winters
- **Embedding-based detector**: предобученные embeddings для HTTP responses; cosine distance от baseline embedding cluster

### 2.3 LLM-детекторы

**Исследование**: оценить эффективность и cost-performance LLM для анализа HTTP responses; бенчмарки local models (Llama, Mistral) vs. API-based (GPT-4o-mini); определить latency budget.

**Реализация**:

- **LLM response analyzer** (batch/async): для каждого finding -- отправка request+response pair в LLM с prompt для классификации (vulnerability type, severity, confidence); результат записывается как enrichment к Finding
- **LLM-based error classification**: автоматическое извлечение root cause из stack traces и error messages в response body; structured output → расширение `FindingDetails`
- **LLM-driven payload generation**: использование LLM для генерации context-aware payloads на основе структуры приложения (знание API schema, типов данных, бизнес-логики) -- новый мутатор `LLMMutator`
- **Configurable LLM backend**: абстрактный интерфейс `LLMProvider` с реализациями для OpenAI API, local Ollama, Anthropic; конфигурация через `CampaignConfig`

---

## 3. Улучшение работы с эндпоинтами

### 3.1 Импорт HAR-файлов

**Исследование**: изучить HAR 1.2 спецификацию (W3C); mapping HAR entries → RecordingSession/Exchange.

**Реализация**:

- **HAR parser** в новом пакете `internal/harconv/`: парсинг HAR JSON, конвертация `entries[].request/response` → `model.Exchange`, группировка по `(scheme, host, port, path)` → `RecordingSession`; обработка `timings` → `DurationMs`; конвертация body `text`/`encoding` → `BodyB64`
- **API endpoint**: `POST /api/v1/recordings/import/har` принимает HAR file (multipart upload); валидация + конвертация + сохранение
- **Frontend**: расширить [ImportDialog.tsx](web/src/components/ImportDialog.tsx) для поддержки HAR формата (автодетект по содержимому)
- **Фильтрация**: при импорте HAR исключать static assets (CSS, JS, images), OPTIONS preflight, favicon; настраиваемые exclude-patterns

### 3.2 Импорт Swagger/OpenAPI

**Исследование**: изучить OpenAPI 3.x spec; стратегия генерации seed recordings из schema (parameter examples, enum values, default values).

**Реализация**:

- **OpenAPI parser** в `internal/openapi/`: парсинг OpenAPI 3.x YAML/JSON; извлечение endpoints, methods, parameters, request bodies, response schemas; использовать `kin-openapi` library
- **Seed generator**: из распаршенной schema генерировать синтетические `RecordingSession` с валидными requests (из examples/defaults) и placeholder responses
- **API endpoint**: `POST /api/v1/recordings/import/openapi` принимает OpenAPI spec file
- **Parameter-aware mutations**: при наличии OpenAPI schema мутаторы знают типы параметров и могут генерировать type-appropriate boundary values (e.g., для integer field -- INT_MAX, negative; для email -- injection в @ разделителе)
- **Swagger 2.0 backward compatibility**: конвертация Swagger 2.0 → OpenAPI 3.x перед обработкой

### 3.3 Автоматический краулинг

**Исследование**: изучить headless browser crawling (Chrome DevTools Protocol через `chromedp` или `rod`) vs. HTTP-only crawling; trade-offs для SPA applications.

**Реализация**:

- **HTTP crawler** в `internal/crawler/`: рекурсивный краулер на основе HTML parsing (colly или custom); extraction ссылок из `<a href>`, `<form action>`, JavaScript-строк; scope-ограничение по domain/path prefix; rate limiting; respect robots.txt (опционально отключаемый)
- **Headless browser crawler**: интеграция с `chromedp`/`rod` для JS-heavy SPA -- запуск headless Chrome, запись intercepted requests через CDP; конвертация в RecordingSession
- **MITM-proxy + crawler combo**: краулер использует MITM-прокси как upstream; весь трафик автоматически записывается; автообнаружение endpoints
- **API endpoint**: `POST /api/v1/crawl` с конфигурацией (target URL, depth, scope, auth config); асинхронный статус (`GET /api/v1/crawl/:id/status`)
- **Login sequence support**: настраиваемый login flow перед краулингом (form-based, cookie injection, header injection)

---

## 4. Поддержка gRPC

**Исследование**: изучить gRPC-Web и gRPC reflection API; стратегии MITM для HTTP/2 и gRPC; protobuf dynamic deserialization.

**Реализация**:

- **gRPC proxy** в `internal/grpc/`: HTTP/2-aware MITM proxy; перехват gRPC unary и streaming calls; десериализация protobuf через reflection API или предоставленный .proto файл
- **gRPC recorder**: запись gRPC exchanges -- method (service/method), serialized request/response messages, metadata (headers), status codes; расширение `model.Exchange` для gRPC metadata (или отдельный `GRPCExchange` тип)
- **gRPC mutators**: мутации protobuf messages -- type substitution, field removal, boundary values для numeric fields, oversized strings, unknown field injection, invalid wire types; новый пакет `internal/mutate/grpc/`
- **gRPC replayer**: отправка мутированных gRPC requests; обработка gRPC status codes и error details
- **gRPC anomaly detection**: расширение детекторов для gRPC-specific anomалий -- UNIMPLEMENTED/INTERNAL status codes, metadata leaks, stream interruptions
- **Proto import**: `POST /api/v1/recordings/import/proto` -- загрузка .proto файлов для schema-aware мутаций; reflection-based discovery если доступен
- **UI**: отображение gRPC exchanges (method, protobuf JSON representation, metadata, gRPC status)

---

## 5. Улучшение UI/UX

### 5.1 Dashboard и визуализация

**Реализация**:

- **Real-time campaign dashboard**: графики в реальном времени (tests/sec, findings rate, latency distribution, status code distribution) через SSE; использовать `recharts` или `chart.js`; расширить [sse.go](internal/api/sse.go) для передачи time-series данных
- **Findings timeline**: визуализация находок во времени кампании; корреляция с мутациями и endpoints
- **Endpoint coverage map**: heatmap показывающий coverage -- какие endpoints протестированы, сколько мутаций, какие находки; tree-view из [EndpointTree.tsx](web/src/components/EndpointTree.tsx) с colour-coded статусом
- **Request/Response diff view**: side-by-side сравнение original seed request с mutated request; syntax highlighting для JSON; расширить [ExchangeViewer.tsx](web/src/components/ExchangeViewer.tsx)
- **Campaign comparison**: сравнение результатов нескольких кампаний по одним и тем же endpoints

### 5.2 UX-улучшения

**Реализация**:

- **Campaign wizard**: пошаговый мастер создания кампании вместо single-form в [CampaignCreatePage.tsx](web/src/pages/CampaignCreatePage.tsx); шаги: выбор recordings → настройка target → mutations config → anomaly config → triage config → review & launch
- **Quick actions**: контекстное меню на findings -- "Reproduce", "View Artifact", "Copy cURL", "Export"
- **cURL export**: генерация cURL команды из Exchange для ручного воспроизведения; frontend + API endpoint
- **Keyboard shortcuts**: навигация по findings (j/k), быстрый фильтр (f), reproduce (r)
- **Notifications**: browser notifications при завершении кампании или обнаружении critical findings; расширить SSE
- **Responsive design**: мобильная адаптация для мониторинга кампаний с телефона
- **Dark mode polish**: улучшение контрастности и консистентности dark theme

### 5.3 Reporting

**Реализация**:

- **Export findings**: PDF/HTML отчёт с summary, findings list, reproduction steps, severity assessment; новый пакет `internal/report/export/`
- **Markdown export**: генерация Markdown отчёта для integration с issue trackers (GitHub Issues, Jira)
- **CSV/JSON export**: bulk export findings для интеграции с другими инструментами
- **Customizable report templates**: шаблоны отчётов с настраиваемыми секциями

---

## 6. Оптимизации производительности

### 6.1 Engine performance

**Исследование**: профилирование текущего bottleneck (pprof CPU/memory profile при кампании на 1000+ recordings); анализ аллокаций и GC pressure.

**Реализация**:

- **Connection pooling**: в [replayer.go](internal/replayer/replayer.go) -- переиспользование HTTP connections через `http.Transport` с настроенными `MaxIdleConnsPerHost`, `IdleConnTimeout`; снизить TCP/TLS handshake overhead
- **Zero-copy mutations**: минимизировать аллокации в mutation pipeline; использовать `sync.Pool` для `MutationResult` и temporary buffers в [mutate.go](internal/mutate/mutate.go)
- **Batch database operations**: в [worker.go](internal/engine/worker.go) -- группировка finding inserts и artifact writes в batches вместо per-finding; reduce database round-trips
- **Adaptive rate limiting**: в [ratelimit.go](internal/engine/ratelimit.go) -- adaptive token bucket реагирующий на SUT response times; автоматическое снижение RPS при detection of SUT overload
- **Corpus sharding**: при большом corpus (10,000+ sessions) -- partition seeds по endpoints и distribute across workers для лучшей cache locality
- **Async artifact writing**: выделить artifact writing в отдельный goroutine pipeline чтобы не блокировать worker loop

### 6.2 Database performance

**Реализация**:

- **Prepared statements**: кэширование prepared statements в store layer для частых queries (finding lookup by signature, recording fetch by ID)
- **Read replicas support**: опциональная конфигурация read replica для тяжёлых read queries (findings list, stats aggregation)
- **Findings partitioning**: PostgreSQL table partitioning для findings table по `campaign_id` -- ускорение queries при большом количестве кампаний
- **Materialized views**: для dashboard stats -- pre-computed aggregations вместо runtime COUNT/SUM
- **Connection pool tuning**: настраиваемый `MaxOpenConns`, `MaxIdleConns`, `ConnMaxLifetime` через config

### 6.3 Frontend performance

**Реализация**:

- **Virtualized lists**: react-window для findings table и recordings list при >1000 элементов
- **Optimistic updates**: instant UI feedback при start/stop campaign
- **SSE reconnection**: улучшить [useSSE.ts](web/src/hooks/useSSE.ts) -- exponential backoff reconnect, connection health indicator
- **Bundle optimization**: code splitting по routes, lazy loading тяжёлых компонентов (ExchangeViewer, JsonViewer)

---

## 7. Триаж находок

### ~~7.1 Улучшение формального триажа~~

~~**Реализация** (расширение [triage.go](internal/triage/triage.go)):~~

- ~~**Severity scoring**: формальная классификация severity (Critical/High/Medium/Low/Info) на основе: finding type, endpoint sensitivity (auth endpoints > static), mutation type, reproducibility rate~~
- ~~**Improved deduplication**: context-aware дедупликация -- учёт не только signature, но и root cause similarity; группировка findings по underlying vulnerability (e.g., все SQLi findings на разных endpoints → одна vulnerability group)~~
- ~~**Payload minimization for non-JSON**: расширить delta-debug алгоритм для query parameters, multipart body, XML body; сейчас MinimizeJSONBody работает только с JSON~~
- ~~**Auto-categorization**: правила классификации findings по vulnerability category (OWASP Top 10 2025 mapping) на основе mutation type + anomaly type + response patterns~~

### 7.2 ML-based триаж

**Исследование**: собрать dataset из findings (true positive vs false positive) по результатам ручных review; оценить минимальный dataset size для обучения.

**Реализация**:

- **False positive classifier**: обучить модель (Random Forest или XGBoost) на features: finding type, endpoint pattern, mutation type, response size delta, status code, baseline correlation, reproducibility rate; binary classification: true positive vs false positive
- **Priority ranker**: ML-based ранжирование findings по likelihood of being exploitable; learning to rank на features из request/response pairs
- **Feedback loop**: UI элемент "Mark as False Positive" / "Confirm Real" → записывается в DB → используется для ре-обучения моделей
- **Clustering similar findings**: unsupervised clustering (DBSCAN на embedding-ах findings) для автоматической группировки related findings

### 7.3 LLM-based триаж

**Реализация**:

- **LLM triage agent**: для каждого finding → отправка (request, mutated_request, baseline_response, anomalous_response) в LLM с structured prompt; output: vulnerability classification, severity, exploitability assessment, recommended remediation
- **Batch LLM analysis**: после завершения кампании -- batch-обработка всех UNCONFIRMED findings через LLM для приоритизации human review
- **Natural language finding description**: LLM генерирует human-readable описание каждого finding (что произошло, почему это может быть уязвимостью, как воспроизвести)
- **LLM-assisted report generation**: генерация executive summary и technical details из набора findings

---

## 8. MCP-сервер

**Исследование**: изучить Model Context Protocol specification; patterns для resource/tool/prompt definition; streaming support.

**Реализация** (новый пакет `internal/mcp/`):

### 8.1 Core MCP server

- **Transport**: stdio и HTTP (SSE) transport; реализация на Go с использованием `mcp-go` SDK
- **Resources**: expose recordings, campaigns, findings, artifacts, campaign stats как MCP resources с URI scheme `ffuuzz://`
- **Tools**:
  - `create_campaign` -- создание кампании с полной конфигурацией
  - `start_campaign` / `stop_campaign` -- управление lifecycle
  - `get_campaign_status` -- текущий статус и статистика
  - `list_findings` -- получение findings с фильтрацией
  - `get_finding_detail` -- детали finding + artifact
  - `reproduce_finding` -- запуск воспроизведения
  - `import_recordings` -- импорт recordings (JSON, HAR, OpenAPI)
  - `export_findings` -- экспорт findings в различных форматах
  - `get_endpoint_tree` -- дерево endpoints
  - `start_crawl` -- запуск краулера
  - `analyze_finding` -- LLM-анализ finding (если LLM backend сконфигурирован)
- **Prompts**: предоставить prompt templates для типичных use-cases (security analysis, triage guidance, report generation)

### 8.2 Integration

- **CLI command**: `ffuuzz mcp` -- запуск standalone MCP server (stdio transport для IDE integration)
- **Embedded mode**: MCP server как часть `ffuuzz serve` (HTTP transport)
- **Authentication**: API key authentication для MCP tools
- **Streaming**: SSE для real-time campaign progress через MCP

---

## 9. Автопилот на базе мультиагентной системы

**Исследование**: изучить архитектуры мультиагентных систем (CrewAI, AutoGen, LangGraph); определить оптимальную agent topology для security testing; оценить cost/performance.

### 9.1 Agent framework

**Реализация** (новый пакет `internal/autopilot/`):

- **Agent runtime**: фреймворк для запуска и координации агентов; message passing, shared state, tool execution; поддержка разных LLM backends
- **Agent types**:
  1. **Planner Agent**: анализирует target application (endpoints, technologies, authentication) и формирует стратегию тестирования
  2. **Recon Agent**: выполняет reconnaissance -- краулинг, OpenAPI discovery, technology fingerprinting, authentication flow analysis
  3. **Fuzzer Agent**: конфигурирует и запускает fuzzing campaigns с оптимальными параметрами для каждого типа endpoint; адаптирует стратегию мутаций на основе промежуточных результатов
  4. **Triage Agent**: анализирует findings, классифицирует severity, определяет exploitability, группирует related findings
  5. **Reporter Agent**: генерирует comprehensive security report из findings

### 9.2 Orchestration

**Реализация**:

- **Pipeline orchestrator**: последовательно-параллельный pipeline: Recon → Planner → [Fuzzer × N] → Triage → Reporter
- **Feedback loops**: Triage Agent может запросить Fuzzer Agent провести дополнительное тестирование конкретного endpoint; Fuzzer Agent адаптирует mutation strategy на основе находок
- **Human-in-the-loop**: опциональные checkpoints где автопилот запрашивает human confirmation перед продолжением (e.g., before aggressive fuzzing, before report generation)
- **Session management**: полный audit trail всех agent actions, decisions, и tool calls

### 9.3 Target-aware intelligence

**Реализация**:

- **Technology detection**: автоматическое определение серверного framework, language, WAF на основе response fingerprints → адаптация mutation strategy
- **Authentication handling**: автоматическое обнаружение и обработка authentication flows (login forms, OAuth, API keys) для authenticated fuzzing
- **Scope management**: автоматическое определение in-scope и out-of-scope endpoints с подтверждением от пользователя
- **Progressive testing**: начать с safe mutations, escalate к более aggressive на основе target resilience

### 9.4 UI integration

**Реализация**:

- **Autopilot dashboard page**: настройка и запуск автопилота; real-time view agent actions и decisions
- **Agent log viewer**: structured log всех agent interactions и tool calls
- **Override controls**: возможность interrupt, redirect, или override agent decisions в реальном времени
- **Chat interface**: natural language interaction с автопилотом -- "сфокусируйся на authentication endpoints", "протестируй injection на /api/users"

---

## Порядок приоритетов

**Фаза 1 -- Foundation**:

- 3.1 HAR import
- 3.2 OpenAPI import
- 2.1 Новые формальные детекторы
- 1.1 Улучшение существующих мутаторов
- 6.1-6.2 Performance optimizations
- 5.2 UX-улучшения (cURL export, campaign wizard)

**Фаза 2 -- Expansion**:

- 1.2 Новые мутаторы (XML, Multipart, Auth, Encoding)
- 3.3 Автоматический краулинг
- 7.1 Улучшение формального триажа
- 5.1 Dashboard и визуализация
- 5.3 Reporting
- 8.1 MCP core server

**Фаза 3 -- Intelligence**:

- 2.2 Статистические и ML-детекторы
- 7.2 ML-based триаж
- 1.3 Stateful-aware мутации
- 4 gRPC support
- 8.2 MCP integration

**Фаза 4 -- Autonomy**:

- 2.3 LLM-детекторы
- 7.3 LLM-based триаж
- 1.2 (continued) GraphQL mutator, Grammar-aware mutator
- 9.1-9.4 Мультиагентный автопилот
