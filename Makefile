.PHONY: help build test lint migrate-up migrate-down seed docker-build docker-up docker-down generate-sqlc generate-proto swagger run clean test-integration test-e2e lint-all docker-build-all docker-push security-scan coverage-report

APP_NAME := complianceforge
API_BINARY := bin/$(APP_NAME)-api
CMD_API := cmd/api/main.go

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

## lint: run golangci-lint
lint:
	golangci-lint run ./...

## migrate-up: apply all pending database migrations
migrate-up:
	go run cmd/migrate/main.go up

## migrate-down: roll back the last database migration
migrate-down:
	go run cmd/migrate/main.go down

## seed: populate the database with seed data
seed:
	go run cmd/seed/main.go

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

## swagger: regenerate Swagger/OpenAPI documentation
swagger:
	swag init -g $(CMD_API) -o docs/swagger

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
	go run cmd/migrate/main.go up
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
	docker build -f deployments/docker/Dockerfile.api -t complianceforge-api:latest .
	docker build -f deployments/docker/Dockerfile.frontend -t complianceforge-frontend:latest ./frontend

## docker-push: tag and push images to GHCR
docker-push:
	@if [ -z "$$REGISTRY" ] || [ -z "$$IMAGE_PREFIX" ] || [ -z "$$TAG" ]; then \
		echo "Usage: make docker-push REGISTRY=ghcr.io IMAGE_PREFIX=<owner/repo> TAG=<tag>"; \
		exit 1; \
	fi
	docker tag complianceforge-api:latest $$REGISTRY/$$IMAGE_PREFIX/api:$$TAG
	docker tag complianceforge-frontend:latest $$REGISTRY/$$IMAGE_PREFIX/frontend:$$TAG
	docker push $$REGISTRY/$$IMAGE_PREFIX/api:$$TAG
	docker push $$REGISTRY/$$IMAGE_PREFIX/frontend:$$TAG

## security-scan: run trivy, gosec, and npm audit
security-scan:
	trivy image --severity HIGH,CRITICAL --exit-code 1 complianceforge-api:latest
	trivy image --severity HIGH,CRITICAL --exit-code 1 complianceforge-frontend:latest
	gosec ./...
	cd frontend && npm audit --audit-level=high

## coverage-report: generate and open Go HTML coverage report
coverage-report:
	go test -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html
	open coverage.html
