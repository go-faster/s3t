test:
	@./go.test.sh
.PHONY: test

test_fast:
	go test ./...
.PHONY: test_fast

build:
	go build -o s3t ./cmd/s3t
.PHONY: build

lint:
	golangci-lint run ./...
.PHONY: lint

tidy:
	go mod tidy
.PHONY: tidy
