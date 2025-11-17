# .PHONY: docker-up docker-down test-e2e

# docker-up:
# 	sudo docker compose up -d --build

# docker-down:
# 	sudo docker compose down -v

# wait-for-health:
# 	./scripts/wait-for-health.sh

# test-e2e: wait-for-health
# 	./e2e-test.sh

# migrate:
#     sudo docker compose run --rm migrate

SHELL := /bin/bash
GO_FILES := $(shell find . -type f -name '*.go' -not -path "./vendor/*")
BIN_NAME := movies-api
BUILD_DIR := bin
PORT ?= 8080
ENV_FILE ?= .env
DC := docker compose
WAIT_SCRIPT := ./scripts/wait-for-health.sh
BASE_URL ?= http://127.0.0.1:$(PORT)
E2E_SCRIPT ?= ./e2e-test.sh

.PHONY: all build run clean tidy fmt lint test docker-build docker-up docker-up-detach docker-down docker-logs docker-ps test-e2e ci-test-e2e

all: build

build:
	@echo ">> building $(BIN_NAME)"
	@mkdir -p $(BUILD_DIR)
	@GOENV=CGO_ENABLED=0 go build -o $(BUILD_DIR)/$(BIN_NAME) ./cmd/server

run:
	@echo ">> running $(BIN_NAME) with env $(ENV_FILE)"
	@ENV_FILE=$(ENV_FILE) PORT=$(PORT) go run ./cmd/server

clean:
	@rm -rf $(BUILD_DIR)

tidy:
	@go mod tidy

fmt:
	@go fmt ./...

lint:
	@echo ">> go vet"
	@go vet ./...
	@echo ">> staticcheck (if installed)"
	@command -v staticcheck >/dev/null 2>&1 && staticcheck ./... || echo "staticcheck not installed, skipping"

test:
	@echo ">> running unit tests"
	@go test ./...

docker-build:
	@$(DC) build

docker-up:
	@echo ">> starting stack (postgres + migrations + app)"
	@$(DC) up --build

docker-up-detach:
	@echo ">> starting stack in background"
	@$(DC) up -d --build

docker-down:
	@$(DC) down -v

docker-logs:
	@$(DC) logs -f movies-api

docker-ps:
	@$(DC) ps

test-e2e:
	@echo ">> waiting for health at $(BASE_URL)"
	@BASE_URL=$(BASE_URL) $(WAIT_SCRIPT)
	@echo ">> executing E2E script $(E2E_SCRIPT)"
	@$(E2E_SCRIPT)

ci-test-e2e: docker-up-detach test-e2e
	@$(DC) down -v || true