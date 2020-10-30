GO ?= go
GOFMT ?= gofmt
DOCKER ?= docker
PACKAGES ?= $(shell $(GO) list ./...)
DOCFILES := $(wildcard docs/*_api.yml)
ROOTDIR:=$(shell dirname $(realpath $(firstword $(MAKEFILE_LIST))))

GOFILES := $(shell find . -name "*.go" -type f -not -path './vendor/*')
SRCFILES := $(filter-out _test.go,$(GOFILES))

BINFILE := bin/deviceconnect
COVERFILE := coverage.txt

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

$(BINFILE): $(SRCFILES)
	$(GO) build -o $@ .

.PHONY: build
build: $(BINFILE)

$(COVERFILE): $(GOFILES)
	$(GO) test -cover -coverprofile=$@ ${TEST_FLAGS} ./... && \
		echo "\n==>\033[32m Ok\033[m\n" || \
		exit 1

.PHONY: test
test: $(COVERFILE)

.PHONY: fmt
fmt:
	$(GOFMT) -s -w $(GOFILES)

.PHONY: lint
lint:
	golangci-lint run -v

.PHONY: clean
clean:
	$(GO) clean -modcache -x -i ./...
	find . -name coverage.txt -delete
	rm bin/*
