GO ?= go
GOFMT ?= gofmt
DOCKER ?= docker
PACKAGES ?= $(shell go list ./... | \
				grep -v '\(vendor\|mock\|test\)' | \
				tr  '\n' ,)
DOCFILES := $(wildcard docs/*_api.yml)
ROOTDIR := $(shell dirname $(realpath $(firstword $(MAKEFILE_LIST))))

DOCKERTAG ?= $(shell git rev-parse --abbrev-ref HEAD)
DOCKERTESTTAG ?= prtest
DOCKERIMAGE ?= mendersoftware/deviceconnect

GOFILES := $(shell find . -name "*.go" -type f -not -path './vendor/*')
SRCFILES := $(filter-out _test.go,$(GOFILES))

BINFILE := bin/deviceconnect
COVERFILE := coverage.txt

.PHONY: all
all: fmt lint docs build

tests/%: docs/%.yml
	rm -r $@; \
	docker run --rm -t -v $(ROOTDIR):$(ROOTDIR) -w $(ROOTDIR) \
		-u $(shell id -u):$(shell id -g) \
		openapitools/openapi-generator-cli:v4.3.1 generate \
		-g python -i $< \
		-c tests/.openapi-generator.yml \
		-o $(dir $@) \
		--additional-properties=packageName=$*

$(BINFILE): $(SRCFILES)
	$(GO) build -o $@ .

$(BINFILE).test: $(GOFILES)
	go test -c -o $(BINFILE).test \
		-cover -covermode atomic \
		-coverpkg $(PACKAGES)

$(COVERFILE): $(GOFILES)
	$(GO) test -cover -coverprofile=$@ ${TEST_FLAGS} ./... && \
		echo "\n==>\033[32m Ok\033[m\n" || \
		exit 1

.PHONY: docs
docs: $(patsubst docs/%.yml,tests/%,$(DOCFILES))

.PHONY: build
build: $(BINFILE)

.PHONY: build-test
build-test: $(BINFILE).test

.PHONY: test
test: $(COVERFILE)

# Dockerfile targets
bin/deviceconnect.docker: Dockerfile $(SRCFILES)
	docker rmi $(DOCKERIMAGE):$(DOCKERTAG) 2>/dev/null; \
	docker build . -f Dockerfile -t $(DOCKERIMAGE):$(DOCKERTAG)
	docker save $(DOCKERIMAGE):$(DOCKERTAG) -o $@

bin/deviceconnect.acceptance.docker: Dockerfile.acceptance $(GOFILES)
	docker rmi $(DOCKERIMAGE):$(DOCKERTESTTAG) 2>/dev/null; \
	docker build . -f Dockerfile.acceptance -t $(DOCKERIMAGE):$(DOCKERTESTTAG) && \
	docker save $(DOCKERIMAGE):$(DOCKERTESTTAG) -o $@

bin/acceptance.docker: tests/Dockerfile
	docker rmi tests 2>/dev/null; \
	docker build tests -f tests/Dockerfile -t testing
	docker save testing -o $@

.PHONY: docker
docker: bin/deviceconnect.docker

.PHONY: docker-test
docker-test: bin/deviceconnect.acceptance.docker

.PHONY: docker-acceptance
docker-acceptance: bin/acceptance.docker

.PHONY: acceptance-tests
acceptance-tests: docker-acceptance docker-test docs
	docker-compose \
		-f tests/docker-compose.acceptance.open-source.yml \
		-p acceptance \
		up -d && \
	docker attach acceptance_tester_1; \
	docker-compose \
		-f tests/docker-compose.acceptance.open-source.yml \
		-p acceptance down


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
	rm -f bin/*
