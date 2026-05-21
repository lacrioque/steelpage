SHELL := /bin/bash

GO       ?= go
NPM      ?= npm
BIN      ?= steelpage
FRONTEND := frontend

.PHONY: all build build-frontend build-backend dev dev-backend dev-frontend clean tidy

all: build

build: build-frontend build-backend

build-frontend:
	cd $(FRONTEND) && $(NPM) install --no-audit --no-fund
	cd $(FRONTEND) && $(NPM) run build

build-backend:
	$(GO) build -o $(BIN) ./cmd/steelpage

dev:
	@echo "Run these in separate terminals:"
	@echo "  make dev-backend"
	@echo "  make dev-frontend"

dev-backend:
	$(GO) run ./cmd/steelpage

dev-frontend:
	cd $(FRONTEND) && $(NPM) run dev

tidy:
	$(GO) mod tidy

clean:
	rm -f $(BIN)
	rm -rf $(FRONTEND)/dist
	mkdir -p $(FRONTEND)/dist
	touch $(FRONTEND)/dist/.gitkeep
