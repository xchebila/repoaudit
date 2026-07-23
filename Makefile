.PHONY: build test check clean

VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)

build:
	go build -ldflags "-X main.version=$(VERSION)" -o repoaudit .

test:
	go test ./...

check:
	go build ./...
	go vet ./...
	gofmt -l .
	go test ./...

clean:
	rm -f repoaudit
