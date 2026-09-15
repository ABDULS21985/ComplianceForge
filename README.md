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
| Language | Go 1.24 |
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

# Apply migrations and the supported, ordered reference-data seeds
make bootstrap-db

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
sql/
  migrations/  Versioned PostgreSQL migrations
  seeds/       Ordered reference-data seeds and manifest
docs/           OpenAPI lifecycle and operating runbooks
scripts/        Helper scripts
```

## Development

```bash
make help          # Show all available targets
make build         # Compile the API binary
make test          # Run tests with race detection
make lint          # Run golangci-lint
make openapi-generate # Regenerate the OpenAPI 3.1 artifact
make openapi-validate # Validate spec, mounted routes, and payload contracts
make migrate-up    # Apply pending migrations
make migrate-down  # Roll back one migration
make seed          # Apply/verify immutable seeds from sql/seeds/manifest.txt
```

`cmd/migrate` prefers the dedicated `MIGRATION_DATABASE_URL`, then retains
`DATABASE_URL` and the application's `CF_DATABASE_*` settings as development
fallbacks. Production API and worker containers require separate
`API_DATABASE_URL` and `WORKER_DATABASE_URL` logins; see
[`docs/runbooks/database-role-separation.md`](docs/runbooks/database-role-separation.md).
The migrator locates migrations in `sql/migrations` (or `MIGRATIONS_PATH`), and
`cmd/seed` applies only files in `sql/seeds/manifest.txt` (or `SEEDS_PATH`) in
declared order. Applied seed checksums and positions are recorded in
`bootstrap_seed_history`; rerunning the command is safe, while changing or
reordering an applied seed fails closed.

Tenant-specific/demo seed packs are intentionally excluded from the bootstrap
manifest and must be provisioned with a real organization context.

## Security governance

Security-sensitive changes follow the
[secure SDLC](docs/runbooks/secure-sdlc.md), update the living
[threat model](docs/security/threat-model.md), and keep residual decisions in
the [architecture risk register](docs/security/architecture-risk-register.md).
The pull-request template records the required threat IDs, failure-path tests,
review independence, and production evidence. Vulnerabilities must be reported
privately under [SECURITY.md](SECURITY.md) and are handled through the
[vulnerability-management runbook](docs/runbooks/vulnerability-management.md).

## License

Proprietary. All rights reserved.
# grc
