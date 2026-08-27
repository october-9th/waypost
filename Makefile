BINARY := albert
PREFIX ?= /usr/local

.PHONY: build install test check clean

# build: binary nằm ở ./bin, tự copy sang $(PREFIX)/bin khi thấy ổn:
#   sudo cp bin/albert /usr/local/bin/
build:
	go build -o bin/$(BINARY) .

# install: bỏ thẳng vào $(go env GOPATH)/bin — tiện nếu thư mục đó đã có
# trong PATH, khỏi cần sudo.
install:
	go install .

test:
	go test ./...

check: test
	gofmt -l .
	go vet ./...

clean:
	rm -rf bin
