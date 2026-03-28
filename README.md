# ComplianceForge

ComplianceForge is a governance, risk, and compliance (GRC) management platform built for European enterprises. It provides a unified workspace for managing control frameworks, conducting risk assessments, tracking audit evidence, and maintaining continuous compliance posture.

## Supported Frameworks

- ISO 27001
- UK GDPR
- NCSC Cyber Assessment Framework (CAF)
- Cyber Essentials / Cyber Essentials Plus
- NIST SP 800-53
- NIST Cybersecurity Framework (CSF) 2.0
- PCI DSS
- ITIL
- COBIT 2019

## Tech Stack

| Layer | Technology |
|-------|------------|
| Language | Go 1.22 |
| HTTP Router | chi |
| Database | PostgreSQL (pgx) |
| Cache | Redis |
| Messaging | RabbitMQ (AMQP 0-9-1) |
| RPC | gRPC / Protocol Buffers |
| Auth | JWT, OAuth 2.0 |
| Config | Viper |
| Logging | zerolog |
| Docs | Swagger / OpenAPI |

## Quick Start

```bash
# Clone the repository
git clone https://github.com/complianceforge/platform.git
cd platform

# Copy environment config
cp .env.example .env

# Start all services
docker compose up -d

# Apply migrations and seed data
make migrate-up
make seed

# The API is now available at http://localhost:8080
```

## Project Structure

```
cmd/
  api/          API server entrypoint
  migrate/      Database migration runner
  seed/         Seed data loader
  worker/       Background job worker
internal/
  config/       Configuration loading
  domain/       Core domain models and interfaces
  handler/      HTTP and gRPC handlers
  middleware/    Auth, logging, rate-limiting middleware
  repository/   Database access layer (sqlc-generated)
  service/      Business logic
  worker/       Async job processors
proto/          Protocol Buffer definitions
migrations/     SQL migration files
docs/           Swagger / OpenAPI output
scripts/        Helper scripts
```

## Development

```bash
make help          # Show all available targets
make build         # Compile the API binary
make test          # Run tests with race detection
make lint          # Run golangci-lint
make swagger       # Regenerate API docs
```

## License

Proprietary. All rights reserved.
# grc
