APP_NAME=my-replica
CMD=./cmd/repl-tool

build:
	go build -o bin/$(APP_NAME) $(CMD)

run:
	go run $(CMD)

build-linux:
	GOOS=linux GOARCH=amd64 go build -o dist/$(APP_NAME)-linux-amd64 $(CMD)

build-windows:
	GOOS=windows GOARCH=amd64 go build -o dist/$(APP_NAME)-windows-amd64.exe $(CMD)

build-all: build-linux build-windows

fmt:
	go fmt ./...

tidy:
	go mod tidy
