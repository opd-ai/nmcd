.PHONY: build clean test fmt vet

build:
	go build -v -o nmcd ./cmd/nmcd
	go build -v -o permamail ./cmd/permamail

clean:
	rm -f nmcd permamail

test:
	go test -v ./...

fmt:
	gofmt -s -w .

vet:
	go vet ./...

all: fmt vet build
