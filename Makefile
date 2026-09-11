

build:
	go build -o bin/my-go-exchange.exe

run: build
	bin/my-go-exchange.exe
