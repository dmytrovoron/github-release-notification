# GitHub Release Notification - Implementation Plan

## Objective

Build a single Go service that implements the API contract from `api/swagger.yaml` and sends email notifications when a tracked GitHub repository publishes a new release.

## Non-Negotiable Constraints

- Do not change API contracts defined in `api/swagger.yaml`.
- Keep a monolith architecture (API + scanner + notifier in one service).
- Use PostgreSQL as the primary database; store all state there.
- Run schema migrations on service startup.
- Validate repositories with GitHub API at subscription time.
- Handle GitHub API rate limits (`429 Too Many Requests`) correctly.
- Provide `Dockerfile` and `docker-compose.yml` to run the full system.
- Include unit tests for business logic.
- Include mandatory end-to-end (e2e) tests for critical user flows.
- Implement e2e tests using `testcontainers-go`.
- Use `golangci-lint` for linting.
- Use `Makefile` targets as the primary task interface.
- Treat GitHub Actions lint/test pipeline as core delivery scope.

## Required API Behavior

- `POST /api/subscribe`
  - Validates `owner/repo` format, otherwise `400`.
  - Calls GitHub API to verify repository exists, otherwise `404`.
  - Creates pending subscription and sends confirmation email with token.
- `GET /api/confirm/{token}`
  - Activates a pending subscription by token.
- `GET /api/unsubscribe/{token}`
  - Deactivates/removes an active subscription by token.
- `GET /api/subscriptions?email={email}`
  - Returns active subscriptions for an email.

## Data Model (Minimum)

Database: PostgreSQL.

- `subscriptions`
  - `id`, `email`, `repository`, `status` (`pending|active|unsubscribed`), `confirm_token`, `unsubscribe_token`, `created_at`, `updated_at`
- `repository_states`
  - `repository` (unique), `last_seen_tag`, `last_checked_at`, `updated_at`
- Optional event log table for notification history (recommended to avoid duplicate sends).

## Delivery Phases

### Phase 1 - Service Foundation

- Create project structure (`cmd`, `internal`, `migrations`).
- Configure HTTP server, PostgreSQL connection, startup migration runner, and config loading.
- Define repository interfaces and service-layer contracts.
- Create `Makefile` baseline targets (at minimum: `make lint`, `make test`, `make run`).

Exit criteria:

- Service starts, connects to DB, applies migrations, exposes health endpoint.

### Phase 2 - Subscription Flows

- Implement handlers for `subscribe`, `confirm`, `unsubscribe`, and `subscriptions`.
- Add input validation for `owner/repo` and email.
- Integrate GitHub repository existence check in subscribe flow.
- Generate/store confirmation and unsubscribe tokens.

Exit criteria:

- Endpoints return expected status codes and payloads per swagger contract.

### Phase 3 - Scanner and Notifier

- Add background scheduler (interval-based poller inside the same process).
- For each active subscription repository:
  - Fetch latest release from GitHub API.
  - Compare with `repository_states.last_seen_tag`.
  - Send email only when tag changes.
  - Persist updated `last_seen_tag`.
- Add idempotency protection so one release does not trigger duplicate notifications.

Exit criteria:

- New release detection works end-to-end and only sends once per new tag.

### Phase 4 - Resilience and Rate Limiting

- Implement GitHub client with:
  - timeout and retry policy for transient failures;
  - explicit handling for `429` (respect reset window/backoff);
  - token-based auth support (if provided) to increase rate limit.
- Add logging around poll failures without crashing scheduler.

Exit criteria:

- Polling remains stable under GitHub throttling and transient API failures.

### Phase 5 - Tests and Containerization

- Write unit tests for service/business logic:
  - subscribe validation paths;
  - token confirmation/unsubscribe behavior;
  - release comparison and notification trigger logic;
  - 429 handling branches.
- Write mandatory e2e tests for critical flows:
  - subscribe -> confirm -> listed in subscriptions;
  - unsubscribe removes/deactivates active subscription;
  - scanner detects new release and triggers notification once per new tag.
  - use `testcontainers-go` to provision PostgreSQL and required test dependencies.
- Provide `Dockerfile` and `docker-compose.yml` for service + DB.
- Configure `golangci-lint` (for example via `.golangci.yml`) and ensure lint is reproducible with `make lint`.
- Add GitHub Actions workflow to run lint and tests on every push/PR as a required core deliverable.

Exit criteria:

- `make lint` and `make test` pass locally.
- `make test` includes both unit and e2e test suites.
- e2e suite runs against `testcontainers-go` managed services.
- CI pipeline executes lint and tests successfully.
- Service and PostgreSQL boot with docker compose.

## Verification Checklist

- [ ] API behavior matches `api/swagger.yaml` exactly.
- [ ] Invalid repository format returns `400`.
- [ ] Missing GitHub repository returns `404`.
- [ ] Migrations run automatically on startup.
- [ ] PostgreSQL is used as the backing database in app and docker compose.
- [ ] Scanner updates/stores `last_seen_tag` per repository.
- [ ] Notifications send only for unseen release tags.
- [ ] GitHub `429` is handled with safe retry/backoff behavior.
- [ ] Unit tests cover business rules.
- [ ] E2E tests cover critical subscription and notification flows.
- [ ] E2E tests are implemented with `testcontainers-go`.
- [ ] `golangci-lint` runs via `make lint`.
- [ ] Tests run via `make test`.
- [ ] GitHub Actions runs lint and tests for push/PR.
- [ ] `Dockerfile` and `docker-compose.yml` run the system.

## Stretch Goals (After Core Completion)

- Redis cache for GitHub API responses (TTL 10 minutes).
- API key auth for protected endpoints.
- Prometheus `/metrics`.
- Hosted deployment and optional subscription UI.
