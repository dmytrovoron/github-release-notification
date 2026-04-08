---
name: github-release-notification-implementation
description: Implement and maintain the GitHub Release Notification monolith in Go while preserving swagger contract compatibility, database-backed state, release polling with last_seen_tag, and required error handling. Use when building endpoints, scanner/notifier logic, migrations, tests, or Docker setup for this repository.
---

# GitHub Release Notification Implementation

## When To Use

Use this skill when working on:

- API handlers for subscription lifecycle.
- GitHub release scanning and email notification logic.
- Database schema and migration changes.
- Test coverage for business logic.
- Docker runtime setup for local execution.

## Mandatory Constraints

- Do not change request/response contracts from `api/swagger.yaml`.
- Keep architecture as a monolith (API + scanner + notifier together).
- Use PostgreSQL as the database for all persisted state.
- Run schema migrations during service startup.
- Enforce `owner/repo` format validation:
  - invalid format -> `400`
  - repository not found in GitHub -> `404`
- Handle GitHub `429 Too Many Requests` explicitly.
- Use `golangci-lint` for linting, invoked via `make lint`.
- Use `make` targets as the default interface for lint/test/build/run.
- Treat GitHub Actions lint + test workflow as top-priority core scope.
- Treat e2e test coverage as a mandatory requirement (not optional).
- Implement e2e tests using `testcontainers-go`.

## Implementation Order

1. Set up service skeleton, config, DB wiring, and startup migrations.
2. Implement subscription endpoints:
   - `POST /api/subscribe`
   - `GET /api/confirm/{token}`
   - `GET /api/unsubscribe/{token}`
   - `GET /api/subscriptions?email={email}`
3. Add GitHub client for repository existence and release lookup.
4. Implement scanner loop for active subscriptions.
5. Implement notifier flow with idempotent release notifications.
6. Add unit tests for business logic.
7. Add mandatory e2e tests for critical API/scanner flows.
8. Add `Makefile` targets (`lint`, `test`, `run`, optionally `ci`).
9. Add GitHub Actions workflow for lint and tests on push/PR.
10. Add `Dockerfile` and `docker-compose.yml` to run service + PostgreSQL.

## Required Business Rules

- On subscribe:
  - validate email and repository format;
  - verify repository exists via GitHub API;
  - create pending subscription with confirmation token.
- On confirm token:
  - activate subscription.
- On unsubscribe token:
  - deactivate/remove subscription.
- For release checks:
  - track per-repository `last_seen_tag`;
  - notify only when current release tag differs from stored one;
  - update `last_seen_tag` after successful processing.

## GitHub API Guidance

- Use authenticated requests when token is available.
- For `429`:
  - detect response;
  - back off or defer polling;
  - avoid tight retry loops;
  - keep service healthy (do not crash workers).

## Testing Checklist

- Unit tests cover:
  - repository format validation;
  - subscribe success and failure branches (`400`, `404`);
  - token confirmation and unsubscribe behavior;
  - release diff detection using `last_seen_tag`;
  - rate-limit handling branches.
- E2E tests cover:
  - subscribe -> confirm -> subscriptions listing;
  - unsubscribe flow;
  - scanner notification trigger path for new tag.
- E2E environment uses `testcontainers-go` (at minimum PostgreSQL containerized in tests).
- Lint uses `golangci-lint` and is wired to `make lint`.
- CI executes lint + tests through GitHub Actions on push/PR.

## Definition of Done

- All required endpoints behave per `api/swagger.yaml`.
- Startup migrations run automatically and safely.
- Scanner/notifier path works end-to-end for active subscriptions.
- Duplicate notifications for the same tag are prevented.
- PostgreSQL is used in local and Dockerized runtime.
- `make lint` and `make test` are the default local verification commands.
- Unit tests for business logic pass.
- E2E tests for critical flows pass.
- E2E tests run via `testcontainers-go` and are stable in CI.
- GitHub Actions lint/test workflow is present and passing.
- Docker-based local run is documented and functional.

## Optional Enhancements (After Core)

- Redis cache for GitHub API responses (10 minute TTL).
- API key protection for endpoints.
- Prometheus metrics endpoint.
