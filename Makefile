GO ?= go
GOFMT ?= gofmt "-s"
DOCKER ?= docker
PACKAGES ?= $(shell $(GO) list ./...)
GOFILES := $(shell find . -name "*.go" -type f -not -path './vendor/*')
DOCFILES := $(wildcard docs/*_api.yml)
ROOTDIR:=$(shell dirname $(realpath $(firstword $(MAKEFILE_LIST))))

.PHONY: all
all: fmt lint docs build

.PHONY: docs
docs: $(patsubst docs/%.yml,tests/%,$(DOCFILES))

tests/%: docs/%.yml
	docker run --rm -t -v $(ROOTDIR):$(ROOTDIR) -w $(ROOTDIR) \
		-u $(shell id -u):$(shell id -g) \
		openapitools/openapi-generator-cli:v4.3.1 generate \
		-g python -i $< \
		-c tests/.openapi-generator.yml \
		-o $(dir $@) \
		--additional-properties=packageName=$*

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
