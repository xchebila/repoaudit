.PHONY: build test check clean

build:
	go build -o repoaudit .

test:
	go test ./...

check:
	go build ./...
	go vet ./...
	gofmt -l .
	go test ./...

clean:
	rm -f repoaudit
