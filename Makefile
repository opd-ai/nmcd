.PHONY: build clean test fmt vet loadtest

build:
	go build -v -o nmcd ./cmd/nmcd
	go build -v -o permamail ./cmd/permamail

loadtest:
	go build -v -o loadtest-tool ./loadtest/cmd

clean:
	rm -f nmcd permamail loadtest-tool

test:
	go test -v ./...

fmt:
	gofmt -s -w .

vet:
	go vet ./...

all: fmt vet build
