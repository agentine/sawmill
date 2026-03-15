.PHONY: install build test lint fmt clean ci bench

install:
	go mod tidy

build:
	go build ./...

test:
	go test -race -count=1 ./...

lint:
	go vet ./...
	golangci-lint run

fmt:
	gofmt -w .

clean:
	go clean
	rm -rf bin/

bench:
	go test -bench=. -benchmem ./...

ci: lint test
