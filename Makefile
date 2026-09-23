.PHONY: build run test md-ingest tidy

build:
	go build -o bin/my-go-exchange.exe .
	go build -o bin/md-ingest.exe ./cmd/md-ingest

md-ingest:
	go build -o bin/md-ingest.exe ./cmd/md-ingest

run: md-ingest
	./bin/md-ingest.exe

test:
	go test ./...

tidy:
	go mod tidy
