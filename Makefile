.PHONY: test build clean

test:
	go test ./...
	go vet ./...

build:
	go build -o bin/starcat ./cmd/starcat

clean:
	go clean
