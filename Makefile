BINARY := albert
PREFIX ?= /usr/local

.PHONY: build install test check clean

# Build binary vào ./bin.
build:
	go build -o bin/$(BINARY) .

# Cài vào $(go env GOPATH)/bin.
install:
	go install .

test:
	go test ./...

check: test
	gofmt -l .
	go vet ./...

clean:
	rm -rf bin
