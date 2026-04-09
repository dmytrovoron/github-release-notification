# Project Directory Structure and Library Plan

## Goal

Create the initial repository skeleton and baseline dependencies for the GitHub release notification monolith, while preserving the API contract in [api/swagger.yaml](api/swagger.yaml) and constraints documented in [DESCRIPTION.md](DESCRIPTION.md), [IMPLEMENTATION_PLAN.md](IMPLEMENTATION_PLAN.md), and [SKILL.md](SKILL.md).

## Target Structure

Use this layout as the initial baseline:

- `/cmd/main.go` - app entrypoint, wiring, startup/shutdown.
- `/internal/config/` - env/config parsing and defaults.
- `/internal/http/`
  - `handlers.go` - endpoint handlers (`subscribe`, `confirm`, `unsubscribe`, `subscriptions`).
  - `middleware.go` - request logging, recovery, common HTTP concerns.
- `/internal/service/` - business logic for subscriptions, release scanning, and notifications.
- `/internal/repository/postgres/` - `sqlx` repository implementations.
- `/internal/github/` - GitHub API client (repo validation, releases, 429 handling).
- `/internal/email/` - email sender abstraction and implementation.
- `/internal/scanner/` - background polling runner.
- `/internal/migrations/` - migration bootstrap helper.
- `/migrations/` - SQL migration files (schema + indexes).
- `/tests/e2e/` - e2e tests using `testcontainers-go`.
- `/tests/support/` - shared test fixtures/bootstrap.
- `/.github/workflows/ci.yml` - mandatory lint/test pipeline.
- `/.golangci.yml` - lint config.
- `/Makefile` - canonical tasks (`lint`, `test`, `test-unit`, `test-integration`, `test-e2e`, `run`, `ci`).
- `/docker-compose.yml` and `/Dockerfile` - local/dev containerized runtime.

## Library Decisions

Approved baseline libraries (minimal and focused):

- **HTTP stack**: pure `net/http` (standard library only)
  - Reason: strict requirement, zero external HTTP framework/router dependency.
- **Database driver**: `github.com/jackc/pgx/v5/stdlib`
  - Reason: modern PostgreSQL driver with solid performance.
- **DB access helper**: `github.com/vinovest/sqlx`
  - Reason: maintained fork with active updates; keeps SQL explicit and lightweight.
- **Migrations**: `github.com/golang-migrate/migrate/v4`
  - Reason: straightforward SQL migration flow, startup-friendly.
- **Logging**: Go `log/slog`
  - Reason: standard library structured logging, no extra dependency.
- **Config**: `github.com/caarlos0/env/v11` (or equivalent tiny env parser)
  - Reason: typed env loading with low complexity.
- **GitHub API client**: `github.com/google/go-github/v84/github` (with custom retry/429 logic)
  - Reason: typed GitHub models/endpoints reduce manual API parsing.
- **Swagger tooling**: `github.com/go-swagger/go-swagger/cmd/swagger`
  - Reason: generate server scaffolding directly from `api/swagger.yaml` while preserving contract fidelity.
- **Testing (unit/assertions)**: Go `testing` + `github.com/stretchr/testify`
- **E2E testing**: `github.com/testcontainers/testcontainers-go`
  - Reason: mandatory isolated Postgres-backed e2e runtime.
- **Lint**: `github.com/golangci/golangci-lint` (via make + CI)

## Make and CI Contract

- `make lint` -> run `golangci-lint`.
- `make generate` -> run `go-swagger` generation from `api/swagger.yaml`.
- `make test-unit` -> run unit tests only.
- `make test-integration` -> run integration tests only.
- `make test-e2e` -> run e2e tests (`testcontainers-go` required).
- `make test` -> run unit + integration + e2e suites.
- `make ci` -> lint + all tests (must match GitHub Actions behavior).
- CI workflow in `/.github/workflows/ci.yml` must execute the same `make ci` path on push/PR.

## Implementation Sequence

1. Scaffold directory tree and placeholder package files.
2. Add `go.mod` baseline and install chosen libraries.
3. Add `go-swagger` generation flow from `api/swagger.yaml` and commit generated server scaffold.
4. Add migration runner + first schema migration in `/migrations/`.
5. Add `Makefile` targets for generate/lint/unit/integration/e2e/ci.
6. Add `golangci-lint` config.
7. Add CI workflow with lint + unit + e2e execution.
8. Add `testcontainers-go` e2e bootstrap helpers.
9. Wire server startup with config, DB, migrations, router, and scanner stubs.

## Acceptance Checks

- Structure exists and compiles with `go test ./...` for scaffold stage.
- `make lint`, `make test-unit`, `make test-integration`, `make test-e2e`, and `make test` are available and functional.
- e2e tests use `testcontainers-go` to provision PostgreSQL.
- CI runs lint and tests on every push/PR.
