.PHONY: test vet lint cover examples-build tidy

test:
	go test ./...

vet:
	go vet ./...

lint:
	golangci-lint run ./...

cover:
	go test -coverprofile=coverage.out ./...
	go tool cover -func=coverage.out

examples-build:
	go test ./examples/...

tidy:
	go mod tidy

