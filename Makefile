.DEFAULT_GOAL := check

.PHONY: fmt fmt-check test test-race lint deps-check check

fmt:
	gofmt -w .

fmt-check:
	@files=$$(gofmt -l .) || exit 1; \
	if [ -n "$$files" ]; then \
		printf 'Unformatted Go files:\n%s\n' "$$files"; \
		exit 1; \
	fi

test:
	go test ./...

test-race:
	go test -race ./...

lint:
	golangci-lint run ./...

deps-check:
	go mod tidy -diff

check: fmt-check deps-check test lint
