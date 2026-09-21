APP ?= mezha
CMD_DIR ?= ./cmd/app
BIN_DIR ?= ./bin
GO ?= go
CGO_ENABLED := 1
PREFIX ?= /usr/local
DESTDIR ?=
INSTALL ?= install

.PHONY: help build install run test test-unit fmt tidy clean

help:
	@echo "Targets:"
	@echo "  make build         Build the CLI into \$(BIN_DIR)/\$(APP)"
	@echo "  make install       Install the CLI into \$(DESTDIR)\$(PREFIX)/bin"
	@echo "  make run           Run the CLI locally"
	@echo "  make test          Run all Go tests (unit)"
	@echo "  make test-unit     Run unit tests only"
	@echo "  make fmt           Format Go sources"
	@echo "  make tidy          Tidy Go modules"
	@echo "  make clean         Remove build artifacts"

build:
	@mkdir -p \$(BIN_DIR)
	CGO_ENABLED=\$(CGO_ENABLED) \$(GO) build -o \$(BIN_DIR)/\$(APP) \$(CMD_DIR)

install: build
	@mkdir -p "$(DESTDIR)$(PREFIX)/bin"
	$(INSTALL) -m 0755 "$(BIN_DIR)/$(APP)" "$(DESTDIR)$(PREFIX)/bin/$(APP)"

run:
	CGO_ENABLED=\$(CGO_ENABLED) \$(GO) run \$(CMD_DIR)

test:
	CGO_ENABLED=\$(CGO_ENABLED) \$(GO) test ./...

test-unit:
	CGO_ENABLED=\$(CGO_ENABLED) \$(GO) test ./cmd/... ./internal/...

fmt:
	\$(GO) fmt ./...

tidy:
	\$(GO) mod tidy

clean:
	rm -rf \$(BIN_DIR)
