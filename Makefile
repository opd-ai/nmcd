.PHONY: build clean test fmt vet

build:
	go build -v -o nmcd ./cmd/nmcd

clean:
	rm -f nmcd

test:
	go test -v ./...

fmt:
	gofmt -s -w .

vet:
	go vet ./...

all: fmt vet build
