//go:build e2e

package e2e

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/brianvoe/gofakeit/v7"
	"github.com/stretchr/testify/require"
)

func TestNotifierE2E(t *testing.T) {
	t.Parallel()

	t.Run("pending subscription receives release notification email", func(t *testing.T) {
		email := gofakeit.Email()
		repositoryName := "golang/go"
		tag := "v1.2.3"
		unsubscribeToken := gofakeit.UUID()

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

		_, err = e.db.ExecContext(
			t.Context(),
			`INSERT INTO repository_states (repository, last_seen_tag, last_checked_at, updated_at)
			 VALUES ($1, $2, NOW(), NOW())
			 ON CONFLICT (repository) DO UPDATE SET last_seen_tag = $2, updated_at = NOW()`,
			repositoryName,
			tag,
		)
		require.NoError(t, err, "insert repository state")

		e.waitForMailpitReleaseEmail(t, email, repositoryName, tag, 30*time.Second)

		var lastNotifiedTag string
		err = e.db.QueryRowContext(
			t.Context(),
			"SELECT last_notified_tag FROM subscriptions WHERE email=$1",
			email,
		).Scan(&lastNotifiedTag)
		require.NoError(t, err, "get last_notified_tag")
		require.Equal(t, tag, lastNotifiedTag, "last_notified_tag should be updated after email sent")
	})
}

func (e *e2e) waitForMailpitReleaseEmail(
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

func (e *e2e) findMailpitReleaseEmail(t *testing.T, recipient, repositoryName, releaseTag string) bool {
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
