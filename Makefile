.PHONY: tidy
tidy:
	@go mod tidy

.PHONY: build
build:
	@go build ./..

.PHONY: test
test:
	@go test ./..
