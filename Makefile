.PHONY: build test lint audit coverage clean install help

# Binary name
BINARY=goperf

# Go parameters
GOCMD=go
GOBUILD=$(GOCMD) build
GOTEST=$(GOCMD) test
GOGET=$(GOCMD) get
GOMOD=$(GOCMD) mod
GOFMT=gofmt

# Build info
VERSION?=$(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS=-ldflags "-X main.version=$(VERSION)"

## help: Show this help message
help:
	@echo "goperf - Performance Pattern Detector for Go"
	@echo ""
	@echo "Usage: make [target]"
	@echo ""
	@echo "Targets:"
	@grep -E '^## ' $(MAKEFILE_LIST) | sed 's/## /  /'

## build: Build the binary
build:
	$(GOBUILD) $(LDFLAGS) -o $(BINARY) .

## install: Install to GOPATH/bin
install:
	$(GOCMD) install $(LDFLAGS) .

## test: Run tests
test:
	$(GOTEST) -v -race ./...

## coverage: Run tests with coverage
coverage:
	$(GOTEST) -v -race -coverprofile=coverage.out -covermode=atomic ./...
	$(GOCMD) tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report: coverage.html"

## lint: Run golangci-lint
lint:
	@which golangci-lint > /dev/null || (echo "Installing golangci-lint..." && go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest)
	golangci-lint run ./...

## fmt: Format code
fmt:
	$(GOFMT) -s -w .

## vet: Run go vet
vet:
	$(GOCMD) vet ./...

## audit: Run goperf on itself (dogfooding)
audit: build
	./$(BINARY) ./...

## audit-strict: Run goperf with strict settings
audit-strict: build
	./$(BINARY) --fail-on=medium ./...

## clean: Remove build artifacts
clean:
	rm -f $(BINARY)
	rm -f $(BINARY)-*
	rm -f coverage.out coverage.html
	rm -f *.prof

## deps: Download dependencies
deps:
	$(GOMOD) download
	$(GOMOD) tidy

## check: Run all checks (test, lint, vet, audit)
check: test lint vet audit
	@echo "All checks passed!"

## release: Build release binaries for all platforms
release: clean
	GOOS=linux GOARCH=amd64 $(GOBUILD) $(LDFLAGS) -o $(BINARY)-linux-amd64 .
	GOOS=linux GOARCH=arm64 $(GOBUILD) $(LDFLAGS) -o $(BINARY)-linux-arm64 .
	GOOS=darwin GOARCH=amd64 $(GOBUILD) $(LDFLAGS) -o $(BINARY)-darwin-amd64 .
	GOOS=darwin GOARCH=arm64 $(GOBUILD) $(LDFLAGS) -o $(BINARY)-darwin-arm64 .
	GOOS=windows GOARCH=amd64 $(GOBUILD) $(LDFLAGS) -o $(BINARY)-windows-amd64.exe .
	@echo "Release binaries built:"
	@ls -la $(BINARY)-*

## docker: Build Docker image
docker:
	docker build -t goperf:$(VERSION) .

## example: Run goperf on examples directory
example: build
	./$(BINARY) ./examples/...
