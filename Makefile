.PHONY: build run clean tidy test test-v test-cover

VERSION ?= 0.1.0-dev
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
LDFLAGS = -ldflags "-X main.version=$(VERSION) -X main.commit=$(COMMIT)"

build:
	@mkdir -p bin
	go build $(LDFLAGS) -o bin/klingond ./cmd/klingond

run: build
	./bin/klingond

clean:
	rm -rf bin/
	rm -f coverage.out

tidy:
	go mod tidy

# Run all tests
test:
	go test ./...

# Run all tests with verbose output
test-v:
	go test -v ./...

# Run tests with coverage report
test-cover:
	go test -coverprofile=coverage.out ./...
	go tool cover -func=coverage.out
	@echo ""
	@echo "For HTML report: go tool cover -html=coverage.out"

# Run with debug logging
debug: build
	./bin/klingond --log-level debug

# Run two local nodes for testing
test-local:
	@echo "Starting node 1 on port 4001..."
	@mkdir -p /tmp/klingon-node1
	./bin/klingond --data-dir /tmp/klingon-node1 --listen /ip4/127.0.0.1/tcp/4001 &
	@sleep 2
	@echo "Starting node 2 on port 4002..."
	@mkdir -p /tmp/klingon-node2
	./bin/klingond --data-dir /tmp/klingon-node2 --listen /ip4/127.0.0.1/tcp/4002
