APP ?= mezha
CMD_DIR ?= ./cmd/app
BIN_DIR ?= ./bin
GO ?= go
CGO_ENABLED := 1
PREFIX ?= $(HOME)/.local
DESTDIR ?=
INSTALL ?= install

.PHONY: help build install run test test-unit fmt tidy clean stage-secretspec-static

help:
	@echo "Targets:"
	@echo "  make build         Build a self-contained CLI into $(BIN_DIR)/$(APP)"
	@echo "  make install       Install the CLI into $(DESTDIR)$(PREFIX)/bin"
	@echo "  make run           Run the CLI locally"
	@echo "  make test          Run all Go tests (unit)"
	@echo "  make test-unit     Run unit tests only"
	@echo "  make fmt           Format Go sources"
	@echo "  make tidy          Tidy Go modules"
	@echo "  make clean         Remove build artifacts"

stage-secretspec-static:
	bash scripts/stage-secretspec-static.sh

build: stage-secretspec-static
	@mkdir -p $(BIN_DIR)
	CGO_ENABLED=$(CGO_ENABLED) GOWORK=$(CURDIR)/.secretspec-static.work $(GO) build -tags static -o $(BIN_DIR)/$(APP) $(CMD_DIR)

install: build
	@mkdir -p "$(DESTDIR)$(PREFIX)/bin"
	$(INSTALL) -m 0755 "$(BIN_DIR)/$(APP)" "$(DESTDIR)$(PREFIX)/bin/$(APP)"

run: stage-secretspec-static
	CGO_ENABLED=$(CGO_ENABLED) GOWORK=$(CURDIR)/.secretspec-static.work $(GO) run -tags static $(CMD_DIR)

test: stage-secretspec-static
	CGO_ENABLED=$(CGO_ENABLED) GOWORK=$(CURDIR)/.secretspec-static.work $(GO) test -tags static ./...

test-unit: stage-secretspec-static
	CGO_ENABLED=$(CGO_ENABLED) GOWORK=$(CURDIR)/.secretspec-static.work $(GO) test -tags static ./cmd/... ./internal/...

fmt:
	$(GO) fmt ./...

tidy:
	$(GO) mod tidy

clean:
	rm -rf $(BIN_DIR) .cache/secretspec .secretspec-static.work
