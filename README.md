# GitHub Release Notification

[![CI](https://github.com/dmytrovoron/github-release-notification/actions/workflows/ci.yml/badge.svg)](https://github.com/dmytrovoron/github-release-notification/actions/workflows/ci.yml)
[![Go Version](https://img.shields.io/badge/go-1.26-00ADD8?logo=go)](https://go.dev/dl/)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)

Monolithic Go service for managing email subscriptions to GitHub repository releases.

The current implementation focuses on subscription lifecycle APIs:
- subscribe to a repository release stream
- confirm subscription via token
- unsubscribe via token
- list active subscriptions for an email

## Architecture

```mermaid
flowchart LR
  User[User or Client] --> API[Go API Service]
  API --> DB[(PostgreSQL)]
  API --> GH[GitHub API]
  API --> SMTP[SMTP Server / Mailpit]
  SMTP --> Inbox[Subscriber Inbox]
```

The API validates repositories against GitHub, stores subscription state in PostgreSQL,
and sends confirmation emails through SMTP.

## API

Base path: `/api`

### Endpoints

- `POST /api/subscribe`
  - Inputs: `email`, `repo` (form fields)
  - Validates repository format (`owner/repo`) and checks repository existence via GitHub API
- `GET /api/confirm/{token}`
  - Activates a pending subscription
- `GET /api/unsubscribe/{token}`
  - Marks subscription as unsubscribed
- `GET /api/subscriptions?email={email}`
  - Returns active subscriptions for an email

OpenAPI contract is defined in `api/swagger.yaml`.

## Prerequisites

- Go 1.26+
- Docker + Docker Compose

## Quick Start (Docker)

1. Start all services:

```bash
docker compose up --build
```

2. Verify API health:

```bash
curl -sS http://localhost:8080/healthz
```

3. Open Mailpit UI to inspect confirmation emails:

- http://localhost:8025

Services started by Compose:
- app: API server on `:8080`
- db: PostgreSQL on `:5432`
- mailpit: SMTP (`:1025`) and UI (`:8025`)

## Local Development

1. Start dependencies:

```bash
docker compose up -d db mailpit
```

2. Export environment variables:

```bash
export DATABASE_URL='postgres://app:app@localhost:5432/app?sslmode=disable'
export MIGRATIONS_PATH='file://migrations'
export SMTP_HOST='localhost'
export SMTP_PORT='1025'
export SMTP_FROM='no-reply@github-release-notification.local'
export APP_BASE_URL='http://localhost:8080'
```

3. Run the app:

```bash
go run ./cmd
```

## Example Requests

Subscribe:

```bash
curl -i -X POST 'http://localhost:8080/api/subscribe' \
  -H 'Content-Type: application/x-www-form-urlencoded' \
  --data-urlencode 'email=user@example.com' \
  --data-urlencode 'repo=golang/go'
```

Confirm (replace token from email):

```bash
curl -i 'http://localhost:8080/api/confirm/<token>'
```

List subscriptions:

```bash
curl -i 'http://localhost:8080/api/subscriptions?email=user@example.com'
```

Unsubscribe (replace token from email):

```bash
curl -i 'http://localhost:8080/api/unsubscribe/<token>'
```

## Configuration

Environment variables consumed by the service:

- `DATABASE_URL` (default: `postgres://app:app@localhost:5432/app?sslmode=disable`)
- `MIGRATIONS_PATH` (default: `file://migrations`)
- `DATABASE_PING_TIMEOUT` (default: `5s`)
- `GITHUB_AUTH_TOKEN` (optional; raises GitHub API rate limit)
- `GITHUB_API_BASE_URL` (default: `https://api.github.com`)
- `GITHUB_API_TIMEOUT` (default: `5s`)
- `SMTP_HOST` (default: `localhost`)
- `SMTP_PORT` (default: `1025`)
- `SMTP_FROM` (default: `no-reply@github-release-notification.local`)
- `SMTP_USERNAME` (optional)
- `SMTP_PASSWORD` (optional)
- `APP_BASE_URL` (default: `http://localhost:8080`)
- `SCHEME` (default: `http`)

## Migrations

Database migrations run automatically on startup.

Migration files are located in `migrations/`.

## Quality Gates

Run formatter/static checks and tests via Make targets:

```bash
make lint
make test
```

Additional useful targets:

```bash
make test-unit
make test-integration
make test-e2e
make govulncheck
```

## Notes

- Confirmation email delivery errors are logged and do not fail subscription creation.
- Current codebase includes core subscription flows and infrastructure. Release scanning/notification scheduling can be added on top of the existing data model and integrations.
