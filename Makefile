.PHONY: lint test govulncheck

lint:
	golangci-lint run ./...

test:
	go test -race ./...

govulncheck:
	go run golang.org/x/vuln/cmd/govulncheck@latest -format json ./... > govulncheck.json
	./scripts/check-govulncheck.sh govulncheck.json
