# FFUUZZ

[![Tests](https://github.com/v0lka/ffuuzz/actions/workflows/tests.yml/badge.svg)](https://github.com/v0lka/ffuuzz/actions/workflows/tests.yml)
![Coverage](https://img.shields.io/badge/Coverage-68.7%25-yellow)

FFUZZ is a web application security testing tool that combines MITM proxy traffic recording with intelligent mutation-based fuzzing. It captures HTTP/HTTPS traffic, replays it with various mutations, and detects anomalies that may indicate security vulnerabilities.

![FFUUZZ Dashboard](img/screenshot-dashboard.png)

## Quick Start

### Prerequisites

- Go 1.25+
- Node.js 20+ (for UI development)
- PostgreSQL 16+ (or use Docker Compose)

### Installation

```bash
git clone git@github.com:v0lka/ffuuzz.git
cd ffuuzz

# Configure your environment (copy and edit .env.example)
cp .env.example .env

# Start PostgreSQL
docker-compose up -d postgres

# Build the application
make build

# Run the server
./ffuuzz serve
```

- Proxy: `http://localhost:8080`
- Web UI: `http://localhost:8081`

### Configuration

FFUZZ loads configuration from `.env`, environment variables, and CLI flags (in that priority). Copy `.env.example` to `.env` to get started with all available options and detailed comments. Variable expansion syntax (`${VAR}`) is supported for referencing other environment variables within the file.

Configuration can also be edited through the Web UI via the **Configuration** page in the left sidebar. The form displays all `FFUUZZ_*` settings grouped by category (Server, Database, Storage, Performance, TLS, Certificate Cache, LLM) with inline validation. Changes are written directly to the `.env` file and take effect on the next server restart.

### Development

```bash
make dev-frontend   # Frontend dev server with HMR
make dev-backend    # Backend via go run
make test           # Run tests with race detector
make lint           # Run linters
```

## Documentation

- **[User Guide](docs/user-guide.md)** -- Installation, configuration, usage workflow, mutation operators, anomaly detection, LLM-assisted triage (with UI controls for single-finding and batch analysis), campaign editing, and full REST API reference.
- **[Contributing](docs/contributing.md)** -- Development setup, project structure, architecture overview, key packages, and guidelines for adding new features.

## License

[License](LICENSE)
