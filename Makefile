.PHONY: build run test test-race docker-up docker-down seed lint tidy

BINARY=bin/server

build:
	go build -o $(BINARY) ./cmd/...

run:
	go run ./cmd/main.go

tidy:
	go mod tidy

test:
	go test ./...

test-race:
	go test -race -count=1 ./...

test-cover:
	go test -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html

lint:
	golangci-lint run ./...

docker-up:
	docker compose -f deployments/docker-compose.yml up -d --build
	@echo "Waiting for services to be healthy..."
	@sleep 5
	@docker compose -f deployments/docker-compose.yml ps

docker-down:
	docker compose -f deployments/docker-compose.yml down -v

docker-logs:
	docker compose -f deployments/docker-compose.yml logs -f api

seed:
	bash scripts/seed.sh

load-test:
	@echo "Running load test: 100 concurrent uploads"
	@TOKEN=$$(curl -s -X POST http://localhost:3000/auth/login \
		-H 'Content-Type: application/json' \
		-d '{"email":"seed@example.com","password":"seed1234"}' | jq -r .token); \
	for i in $$(seq 1 100); do \
		curl -s -X POST http://localhost:3000/upload \
			-H "Authorization: Bearer $$TOKEN" \
			-F "file=@testdata/sample.jpg" & \
	done; wait
	@echo "Load test complete"
