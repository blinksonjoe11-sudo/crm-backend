.PHONY: run build test lint docker-up docker-down migrate-up migrate-down swagger tidy

run:
	go run ./cmd/server

build:
	CGO_ENABLED=0 go build -ldflags="-w -s" -o bin/crm ./cmd/server

test:
	go test ./... -race -count=1 -timeout=60s

test-cover:
	go test ./... -coverprofile=coverage.out
	go tool cover -html=coverage.out -o coverage.html

lint:
	golangci-lint run ./...

tidy:
	go mod tidy

docker-up:
	docker compose up -d

docker-down:
	docker compose down

docker-logs:
	docker compose logs -f api

migrate-up:
	migrate -path migrations -database "$$DATABASE_URL" up

migrate-down:
	migrate -path migrations -database "$$DATABASE_URL" down 1

migrate-create:
	migrate create -ext sql -dir migrations -seq $(name)

swagger:
	swag init -g cmd/server/main.go -o docs --parseDependency

env:
	cp .env.example .env
	@echo "Edit .env and fill in your secrets"
