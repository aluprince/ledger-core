.PHONY: run build test migrate-up migrate-down docker-up docker-down tidy lint

# Local dev
run:
	go run ./cmd/api

build:
	CGO_ENABLED=0 go build -ldflags="-s -w" -o bin/ledger-core ./cmd/api

test:
	go test ./... -v -race -count=1

tidy:
	go mod tidy

# Docker
docker-up:
	docker compose up -d

docker-down:
	docker compose down

docker-logs:
	docker compose logs -f api

# Migrations (requires golang-migrate installed)
migrate-up:
	migrate -path db/migrations -database "$$DATABASE_URL" up

migrate-down:
	migrate -path db/migrations -database "$$DATABASE_URL" down

migrate-status:
	migrate -path db/migrations -database "$$DATABASE_URL" version

# sqlc code generation
sqlc-gen:
	sqlc generate --file db/sqlc.yaml

# Lint (requires golangci-lint)
lint:
	golangci-lint run ./...
