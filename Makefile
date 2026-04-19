.PHONY: build run test fmt

build:
	go build ./...

run:
	go run main.go --config relay.example.json

test:
	go test ./...

fmt:
	gofmt -w main.go cmd/relay/main.go
	gofmt -w $$(find internal -name '*.go')
