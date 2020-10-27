GO ?= go
GOFMT ?= gofmt "-s"
DOCKER ?= docker
PACKAGES ?= $(shell $(GO) list ./...)
GOFILES := $(shell find . -name "*.go" -type f -not -path './vendor/*')

.PHONY: all
all: fmt lint test

.PHONY: build
build:
	$(GO) build -o bin/deviceconnect .

.PHONY: test
test:
	$(GO) test -cover -coverprofile=coverage.txt $(PACKAGES) && echo "\n==>\033[32m Ok\033[m\n" || exit 1

.PHONY: test-short
test-short:
	$(GO) test -cover -coverprofile=coverage.txt --short $(PACKAGES) && echo "\n==>\033[32m Ok\033[m\n" || exit 1

.PHONY: fmt
fmt:
	$(GOFMT) -w $(GOFILES)

.PHONY: lint
lint:
	golangci-lint run -v

.PHONY: clean
clean:
	$(GO) clean -modcache -x -i ./...
	find . -name coverage.txt -delete
	rm bin/*
