.PHONY: build test lint clean run-apiaryd run-apiaryctl run-keeper

# Build all binaries
build:
	go build -o bin/apiaryd ./cmd/apiaryd
	go build -o bin/apiaryctl ./cmd/apiaryctl
	go build -o bin/keeper ./cmd/keeper

# Run tests
test:
	go test -v -race -coverprofile=coverage.out ./...

# Run tests with coverage
test-coverage: test
	go tool cover -html=coverage.out -o coverage.html

# Run linter
lint:
	golangci-lint run

# Clean build artifacts
clean:
	rm -rf bin/
	rm -f coverage.out coverage.html

# Run apiaryd (Queen daemon)
run-apiaryd: build
	./bin/apiaryd

# Run apiaryctl CLI
run-apiaryctl: build
	./bin/apiaryctl

# Run keeper
run-keeper: build
	./bin/keeper

# Install dependencies
deps:
	go mod download
	go mod tidy

# Format code
fmt:
	go fmt ./...

# Vet code
vet:
	go vet ./...
