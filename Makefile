.PHONY: build test clean fmt vet

build:
	go build -v ./...

test:
	go test -v ./...

test-short:
	go test -short ./...

bench:
	go test -bench=. -benchmem ./...

fmt:
	go fmt ./...

vet:
	go vet ./...

clean:
	go clean

check: fmt vet test
