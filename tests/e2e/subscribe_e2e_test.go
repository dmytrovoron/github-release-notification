//go:build e2e

package e2e

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"testing"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type e2eEnv struct {
	client             *http.Client
	baseURL            string
	databaseURLForTest string
}

func TestSubscribeEndpointE2E(t *testing.T) {
	env := setupE2EEnv(t)
	email := "alice@example.com"
	repo := "owner/repo"

	postSubscribe(t, env.client, env.baseURL, email, repo, http.StatusOK)
	postSubscribe(t, env.client, env.baseURL, email, repo, http.StatusConflict)

	activateSubscriptionByEmail(t, env.databaseURLForTest, email)

	expectedItems := []subscriptionDTO{
		{
			Email:     email,
			Repo:      repo,
			Confirmed: true,
		},
	}
	actualItems := getSubscriptions(t, env.client, env.baseURL, email)
	assert.Equal(t, expectedItems, actualItems)
}

type subscriptionDTO struct {
	Email     string `json:"email"`
	Repo      string `json:"repo"`
	Confirmed bool   `json:"confirmed"`
}

func postSubscribe(t *testing.T, client *http.Client, baseURL, email, repo string, expectedCode int) {
	t.Helper()

	form := url.Values{}
	form.Set("email", email)
	form.Set("repo", repo)

	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, baseURL+"/subscribe", strings.NewReader(form.Encode()))
	require.NoError(t, err, "build subscribe request")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := client.Do(req)
	require.NoError(t, err, "do subscribe request")
	defer func() {
		_ = resp.Body.Close()
	}()

	require.Equal(t, expectedCode, resp.StatusCode, "unexpected subscribe status")
}

func getSubscriptions(t *testing.T, client *http.Client, baseURL, email string) []subscriptionDTO {
	t.Helper()

	query := url.Values{}
	query.Set("email", email)

	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, baseURL+"/subscriptions?"+query.Encode(), http.NoBody)
	require.NoError(t, err, "build subscriptions request")

	resp, err := client.Do(req)
	require.NoError(t, err, "do subscriptions request")
	defer func() {
		_ = resp.Body.Close()
	}()

	require.Equal(t, http.StatusOK, resp.StatusCode, "unexpected subscriptions status")

	var payload []subscriptionDTO
	err = json.NewDecoder(resp.Body).Decode(&payload)
	require.NoError(t, err, "decode subscriptions response")

	return payload
}

func activateSubscriptionByEmail(t *testing.T, databaseURL, email string) {
	t.Helper()

	db, err := sql.Open("pgx", databaseURL)
	require.NoError(t, err, "open db")
	defer func() {
		_ = db.Close()
	}()

	ctx := t.Context()

	err = db.PingContext(ctx)
	require.NoError(t, err, "ping db")

	_, err = db.ExecContext(ctx, "UPDATE subscriptions SET status='active' WHERE email=$1", email)
	require.NoError(t, err, "activate subscription")
}
