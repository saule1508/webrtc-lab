# Makefile for a Go project
# Adjust BIN_NAME and PKG as necessary

# Binary name
BIN_NAME=signaling-server

# Package name
PKG=./...

# Go files
GOFMT_FILES?=$$(find . -name '*.go' | grep -v vendor)

# Go binary
GO=go

# Prettify gofmt errors
fmt:
	@echo "Running gofmt..."
	@gofmt -s -l -w $(GOFMT_FILES)

# Lint your code
# you need to have golang-ci-lint and golint installed in your PATH
# go get -u golang.org/x/lint/golint
# go get -u github.com/golangci/golangci-lint/cmd/golangci-lint or https://golangci-lint.run/welcome/install/#local-installation
lint:
	#@echo "Running golint..."
	#@$(GOPATH)/bin/golint -set_exit_status $(PKG)
	@echo "Running golangci-lint..."
	@golangci-lint run

# Run tests and generate a coverage report
test: 
	@echo "Running tests..."
	@$(GO) test -v -cover $(PKG)

# Using CGO_ENABLED=0 to build a static binary, otherwise issue with glibc
build: 
	@echo "Building $(BIN_NAME)..."
	@test -d bin || mkdir bin
	@CGO_ENABLED=0 $(GO) build -o bin/$(BIN_NAME)

all: fmt lint test build

.PHONY: fmt lint test build all
