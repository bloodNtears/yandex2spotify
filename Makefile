test:
	@go test -v -tags dynamic -coverprofile=cover.out ./...
	@go tool cover -func=cover.out

# golangci-lint is required
lint:
	golangci-lint --version; \
	golangci-lint run -v --fix