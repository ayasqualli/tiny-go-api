.PHONY: run test fmt vet check demo

run:
	go run .

test:
	go test ./...

fmt:
	gofmt -w *.go

vet:
	go vet ./...

check:
	@test -z "$$(gofmt -l *.go)" || (echo "Run 'make fmt' first"; gofmt -l *.go; exit 1)
	go vet ./...
	go test -race ./...

demo:
	./scripts/demo.sh
