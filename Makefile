BINARY_NAME := treehouse
VERSION     := $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS     := -s -w -X main.version=$(VERSION)

.PHONY: build test lint install

build:
	go build -ldflags "$(LDFLAGS)" -o $(BINARY_NAME) ./cmd/treehouse

test:
	go test ./...

lint:
	@unformatted=$$(gofmt -l .); \
	if [ -n "$$unformatted" ]; then \
		echo "gofmt: the following files need formatting:"; \
		echo "$$unformatted"; \
		exit 1; \
	fi
	go vet ./...

install:
	go install -ldflags "$(LDFLAGS)" ./cmd/treehouse
