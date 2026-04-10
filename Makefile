.PHONY: lint test test-unit test-e2e govulncheck

lint:
	golangci-lint run ./...

test:
	$(MAKE) test-unit
	$(MAKE) test-e2e

test-unit:
	go test -race ./...

test-e2e:
	go test -tags e2e ./tests/e2e/...

govulncheck:
	go run golang.org/x/vuln/cmd/govulncheck@latest -format json ./... > govulncheck.json
	./scripts/check-govulncheck.sh govulncheck.json
