.PHONY: ci lint test test-unit test-e2e test-integration govulncheck tidy

# sync with ci.yml
GOLANGCI_LINT_VERSION := "v2.11.4"
BIN_DIR := "./bin"

ci: tidy fix lint test

tidy:
	go mod tidy -diff
	cd tools && go mod tidy -diff

fix:
	go fix ./... && git diff --exit-code

lint-install:
	@test -x $(BIN_DIR)/golangci-lint || curl -sSfL https://golangci-lint.run/install.sh | sh -s -- -b "$(BIN_DIR)" "$(GOLANGCI_LINT_VERSION)"

lint: lint-install
	$(BIN_DIR)/golangci-lint run ./...

test: test-unit test-integration test-e2e

test-unit:
	go test -race -shuffle=on ./...

test-e2e:
	go test -race -shuffle=on -tags e2e ./tests/e2e/...

test-integration:
	go test -race -shuffle=on -tags integration ./...

govulncheck:
	go run golang.org/x/vuln/cmd/govulncheck@latest -format json ./... > govulncheck.json
	./scripts/check-govulncheck.sh govulncheck.json
