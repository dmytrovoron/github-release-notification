//go:build e2e

package e2e_test

import (
	"database/sql"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/brianvoe/gofakeit/v7"
	"github.com/stretchr/testify/require"
)

var eNotifier e2eNotifier

func TestNotifierE2E(t *testing.T) {
	t.Parallel()

	t.Run("pending subscription receives release notification email", func(t *testing.T) {
		email := gofakeit.Email()
		repositoryName := fakeRepo()
		tag := "v1.2.3"
		unsubscribeToken := gofakeit.UUID()

		eNotifier.insertActiveSubscription(t, email, repositoryName, unsubscribeToken)
		eNotifier.upsertRepositoryStateTag(t, repositoryName, tag)

		eNotifier.waitForMailpitReleaseEmail(t, email, repositoryName, tag, 30*time.Second)

		lastNotifiedTag := eNotifier.getLastNotifiedTagByEmail(t, email)
		require.Equal(t, tag, lastNotifiedTag, "last_notified_tag should be updated after email sent")
	})
}

type e2eNotifier struct {
	smtpAPIBaseURL string
	db             *sql.DB
}

func (e *e2eNotifier) insertActiveSubscription(t *testing.T, email, repositoryName, unsubscribeToken string) {
	t.Helper()

	_, err := e.db.ExecContext(
		t.Context(),
		`INSERT INTO subscriptions (email, repository, status, confirm_token, unsubscribe_token)
		 VALUES ($1, $2, 'active', $3, $4)`,
		email,
		repositoryName,
		gofakeit.UUID(),
		unsubscribeToken,
	)
	require.NoError(t, err, "insert subscription")
}

func (e *e2eNotifier) upsertRepositoryStateTag(t *testing.T, repositoryName, tag string) {
	t.Helper()

	_, err := e.db.ExecContext(
		t.Context(),
		`INSERT INTO repository_states (repository, last_seen_tag, last_checked_at, updated_at)
		 VALUES ($1, $2, NOW(), NOW())
		 ON CONFLICT (repository) DO UPDATE SET last_seen_tag = $2, updated_at = NOW()`,
		repositoryName,
		tag,
	)
	require.NoError(t, err, "insert repository state")
}

func (e *e2eNotifier) getLastNotifiedTagByEmail(t *testing.T, email string) string {
	t.Helper()

	var lastNotifiedTag string
	err := e.db.QueryRowContext(
		t.Context(),
		"SELECT last_notified_tag FROM subscriptions WHERE email=$1",
		email,
	).Scan(&lastNotifiedTag)
	require.NoError(t, err, "get last_notified_tag")

	return lastNotifiedTag
}

func (e *e2eNotifier) waitForMailpitReleaseEmail(
	t *testing.T,
	recipient string,
	repositoryName string,
	releaseTag string,
	timeout time.Duration,
) {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if e.findMailpitReleaseEmail(t, recipient, repositoryName, releaseTag) {
			return
		}

		time.Sleep(2 * time.Second)
	}

	t.Fatalf("timed out waiting for release email to %s for %s tag %s", recipient, repositoryName, releaseTag)
}

func (e *e2eNotifier) findMailpitReleaseEmail(t *testing.T, recipient, repositoryName, releaseTag string) bool {
	t.Helper()

	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, e.smtpAPIBaseURL+"/api/v1/messages", http.NoBody)
	require.NoError(t, err, "create mailpit request")

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err, "request mailpit messages")
	defer func() { _ = resp.Body.Close() }()
	require.Equal(t, http.StatusOK, resp.StatusCode, "mailpit messages status")

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err, "read mailpit response")

	//nolint:tagliatelle // mailpit API returns "Subject" and "To" fields with capitalized first letter
	var payload struct {
		Messages []struct {
			Subject string `json:"Subject"`
			To      []struct {
				Address string `json:"Address"`
			} `json:"To"`
		} `json:"messages"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Logf("mailpit payload decode failed, body=%s", string(body))

		return false
	}

	expectedSubject := "New release for " + repositoryName + ": " + releaseTag
	for i := range payload.Messages {
		msg := payload.Messages[i]
		if msg.Subject != expectedSubject {
			continue
		}
		for j := range msg.To {
			if strings.EqualFold(msg.To[j].Address, recipient) {
				return true
			}
		}
	}

	return false
}

// fakeRepo generates a random non-existent repository.
func fakeRepo() string {
	return strings.ToLower(gofakeit.SafeColor() + "/" + gofakeit.Color())
}
