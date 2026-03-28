.PHONY: help build test lint migrate-up migrate-down seed docker-build docker-up docker-down generate-sqlc generate-proto swagger run clean

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
