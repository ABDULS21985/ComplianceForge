#!/bin/sh
set -eu

SCRIPT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
REPOSITORY_ROOT=$(CDPATH= cd -- "$SCRIPT_DIR/.." && pwd)
cd "$REPOSITORY_ROOT"

export GOTOOLCHAIN=${GOTOOLCHAIN:-go1.26.8}

go run ./cmd/openapi check
go test -count=1 ./internal/openapi
go test -count=1 -run '^TestOpenAPIContract' ./internal/router

cd frontend
npm run type-check
npm test -- --project node src/lib/openapi-contract.test.ts
