.PHONY: help build test test-fuzz-smoke lint migrate-up migrate-down seed bootstrap-db validate-migrations validate-runtime-roles validate-security-governance docker-build docker-up docker-down generate-sqlc generate-proto openapi-generate openapi-validate swagger run clean test-integration test-e2e lint-all docker-build-all docker-push security-tools security-scan backup-db restore-db restore-drill coverage-report

APP_NAME := complianceforge
API_BINARY := bin/$(APP_NAME)-api
CMD_API := cmd/api/main.go
SECURITY_TOOLS_DIR ?= $(CURDIR)/.cache/security-tools/bin
SECURITY_REPORT_DIR ?= $(CURDIR)/security-reports

## help: show this help message (default target)
help:
	@echo "ComplianceForge - GRC Compliance Management Platform"
	@echo ""
	@echo "Usage:"
	@echo "  make <target>"
	@echo ""
	@echo "Targets:"
	@grep -E '^## ' $(MAKEFILE_LIST) | sed 's/## /  /'

## build: compile the API binary
build:
	@echo "Building $(APP_NAME)..."
	CGO_ENABLED=0 go build -ldflags="-s -w" -o $(API_BINARY) $(CMD_API)

## test: run all tests with race detection
test:
	go test -race -cover ./...

## test-fuzz-smoke: run bounded fuzzing of security and state-machine boundaries
test-fuzz-smoke:
	./scripts/run-fuzz-smoke.sh

## lint: run golangci-lint
lint:
	golangci-lint run ./...

## migrate-up: apply all pending database migrations
migrate-up:
	go run ./cmd/migrate up

## migrate-down: roll back the last database migration
migrate-down:
	go run ./cmd/migrate down -steps 1

## seed: populate the database with seed data
seed:
	go run ./cmd/seed

## bootstrap-db: apply migrations and the ordered reference-data seed manifest
bootstrap-db: migrate-up seed

## validate-migrations: verify contiguous reversible migrations and the complete seed manifest
validate-migrations:
	./scripts/validate-migrations.sh

## validate-runtime-roles: prove convergent grants and split-role API/worker behavior on a disposable migrated database
validate-runtime-roles:
	./scripts/validate-runtime-database-roles.sh

## validate-security-governance: verify threat/risk traceability and secure-SDLC review gates
validate-security-governance:
	./scripts/validate-security-governance.sh

## docker-build: build Docker images
docker-build:
	docker compose build

## docker-up: start all services via Docker Compose
docker-up:
	docker compose up -d

## docker-down: stop and remove all Docker Compose services
docker-down:
	docker compose down -v

## generate-sqlc: generate Go code from SQL queries via sqlc
generate-sqlc:
	sqlc generate

## generate-proto: compile Protocol Buffer definitions
generate-proto:
	protoc --go_out=. --go-grpc_out=. proto/**/*.proto

## openapi-generate: regenerate the deterministic OpenAPI 3.1 artifact
openapi-generate:
	go run ./cmd/openapi generate

## openapi-validate: validate generated spec, mounted routes, and Go/frontend contracts
openapi-validate:
	./scripts/validate-openapi.sh

## swagger: backward-compatible alias for OpenAPI generation
swagger: openapi-generate

## run: build and run the API server locally
run: build
	./$(API_BINARY)

## clean: remove build artefacts
clean:
	rm -rf bin/
	go clean -cache

## test-integration: run integration tests with Docker-backed dependencies
test-integration:
	docker compose -f deployments/docker/docker-compose.yml up -d postgres redis
	go test -tags=integration -race ./...
	docker compose -f deployments/docker/docker-compose.yml down

## test-e2e: run Playwright E2E suite against local services
test-e2e:
	docker compose -f deployments/docker/docker-compose.yml up -d postgres redis
	go run ./cmd/migrate up
	go run ./cmd/seed
	cd frontend && npm ci && npm run build
	go run cmd/api/main.go &
	cd frontend && npm run start -- --port 3000 &
	cd frontend && npm run e2e
	pkill -f "cmd/api/main.go" || true
	pkill -f "next start" || true
	docker compose -f deployments/docker/docker-compose.yml down

## lint-all: run backend and frontend lint checks
lint-all:
	golangci-lint run ./...
	cd frontend && npm ci && npm run lint && npm run format:check

## docker-build-all: build backend and frontend images
docker-build-all:
	docker build --target api -f deployments/docker/Dockerfile.api -t complianceforge-api:scan .
	docker build --target worker -f deployments/docker/Dockerfile.api -t complianceforge-worker:scan .
	docker build --target migrator -f deployments/docker/Dockerfile.api -t complianceforge-migrator:scan .
	docker build -f deployments/docker/Dockerfile.frontend -t complianceforge-frontend:scan ./frontend

## docker-push: tag and push images to GHCR
docker-push:
	@if [ -z "$$REGISTRY" ] || [ -z "$$IMAGE_PREFIX" ] || [ -z "$$TAG" ]; then \
		echo "Usage: make docker-push REGISTRY=ghcr.io IMAGE_PREFIX=<owner/repo> TAG=<tag>"; \
		exit 1; \
	fi
	@for component in api worker migrator frontend; do \
		docker tag complianceforge-$$component:scan $$REGISTRY/$$IMAGE_PREFIX/$$component:$$TAG; \
		docker push $$REGISTRY/$$IMAGE_PREFIX/$$component:$$TAG; \
	done

## security-tools: install checksum-pinned scanners outside the application module
security-tools:
	TOOLS_DIR=$(SECURITY_TOOLS_DIR) ./scripts/ci/install-security-tools.sh

## security-scan: scan source/history/dependencies, generate SBOMs, and gate every runtime image
security-scan: security-tools docker-build-all
	mkdir -p $(SECURITY_REPORT_DIR)/sbom
	PATH=$(SECURITY_TOOLS_DIR):$$PATH gitleaks git . --redact --no-banner --exit-code 1 --report-format sarif --report-path $(SECURITY_REPORT_DIR)/gitleaks.sarif
	PATH=$(SECURITY_TOOLS_DIR):$$PATH gosec -fmt=sarif -out=$(SECURITY_REPORT_DIR)/gosec.sarif ./...
	PATH=$(SECURITY_TOOLS_DIR):$$PATH govulncheck -format text -show version ./... > $(SECURITY_REPORT_DIR)/govulncheck.txt
	cd frontend && npm ci --ignore-scripts --no-audit
	cd frontend && npm audit --audit-level=high --json > $(SECURITY_REPORT_DIR)/npm-audit.json
	PATH=$(SECURITY_TOOLS_DIR):$$PATH trivy fs . --scanners vuln,misconfig,secret --include-dev-deps --skip-dirs .git --skip-dirs frontend/node_modules --skip-dirs frontend/.next --severity HIGH,CRITICAL --exit-code 1 --format json --output $(SECURITY_REPORT_DIR)/trivy-filesystem.json
	PATH=$(SECURITY_TOOLS_DIR):$$PATH trivy fs frontend --scanners license --license-full --include-dev-deps --skip-dirs .next --severity HIGH,CRITICAL --exit-code 1 --format json --output $(SECURITY_REPORT_DIR)/trivy-licenses.json
	@set -e; for component in api worker migrator frontend; do \
		PATH=$(SECURITY_TOOLS_DIR):$$PATH syft complianceforge-$$component:scan --output spdx-json=$(SECURITY_REPORT_DIR)/sbom/$$component.spdx.json; \
		PATH=$(SECURITY_TOOLS_DIR):$$PATH trivy image complianceforge-$$component:scan --scanners vuln --severity HIGH,CRITICAL --exit-code 1 --format json --output $(SECURITY_REPORT_DIR)/trivy-$$component.json; \
	done

## backup-db: create an encrypted, verified PostgreSQL logical backup
backup-db:
	./scripts/postgres-backup.sh

## restore-db: restore and verify a backup into an explicitly confirmed target database
restore-db:
	./scripts/postgres-restore.sh

## restore-drill: create, verify, and remove an isolated PostgreSQL restore target
restore-drill:
	./scripts/postgres-restore-drill.sh

## observability-validate: validate Prometheus, OTel Collector, and Grafana configuration
observability-validate:
	./scripts/validate-observability.sh

## coverage-report: generate and open Go HTML coverage report
coverage-report:
	go test -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html
	open coverage.html
