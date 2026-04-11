//go:build e2e

package e2e

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/brianvoe/gofakeit/v7"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	// https://github.com/octocat/Hello-World
	realRepo = "octocat/Hello-World"
)

type e2eEnv struct {
	client             *http.Client
	baseURL            string
	databaseURLForTest string
}

func TestSubscribeEndpointE2E(t *testing.T) {
	env := setupE2EEnv(t)

	t.Run("Subscription successful", func(t *testing.T) {
		email := gofakeit.Email()

		postSubscribe(t, env.client, env.baseURL, email, realRepo, http.StatusOK)

		activateSubscriptionByEmail(t, env.databaseURLForTest, email)

		expectedItems := []subscriptionDTO{
			{
				Email:     email,
				Repo:      realRepo,
				Confirmed: true,
			},
		}
		actualItems := getSubscriptions(t, env.client, env.baseURL, email)
		assert.Equal(t, expectedItems, actualItems)
	})

	t.Run("Invalid input", func(t *testing.T) {
		postSubscribe(t, env.client, env.baseURL, "invalid-email", realRepo, http.StatusBadRequest)
		postSubscribe(t, env.client, env.baseURL, gofakeit.Email(), "invalid/repo/name/with/slashes", http.StatusBadRequest)
	})

	t.Run("Repository not found on GitHub", func(t *testing.T) {
		email := gofakeit.Email()

		postSubscribe(t, env.client, env.baseURL, email, "non-existing/repo-for-test", http.StatusNotFound)
	})

	t.Run("Email already subscribed to this repository", func(t *testing.T) {
		email := gofakeit.Email()

		postSubscribe(t, env.client, env.baseURL, email, realRepo, http.StatusOK)
		postSubscribe(t, env.client, env.baseURL, email, realRepo, http.StatusConflict)
	})
}

func TestConfirmEndpointE2E(t *testing.T) {
	env := setupE2EEnv(t)

	t.Run("Confirm subscription successful", func(t *testing.T) {
		email := gofakeit.Email()

		postSubscribe(t, env.client, env.baseURL, email, realRepo, http.StatusOK)

		token := getConfirmTokenByEmail(t, env.databaseURLForTest, email)

		getConfirm(t, env.client, env.baseURL, token, http.StatusOK)

		expectedItems := []subscriptionDTO{
			{
				Email:     email,
				Repo:      realRepo,
				Confirmed: true,
			},
		}
		actualItems := getSubscriptions(t, env.client, env.baseURL, email)
		assert.Equal(t, expectedItems, actualItems)
	})

	t.Run("Token not found", func(t *testing.T) {
		getConfirm(t, env.client, env.baseURL, "nonexistenttoken123", http.StatusNotFound)
	})

	t.Run("Empty token", func(t *testing.T) {
		getConfirm(t, env.client, env.baseURL, "%20", http.StatusBadRequest)
	})
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

func getConfirm(t *testing.T, client *http.Client, baseURL, token string, expectedCode int) {
	t.Helper()

	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, baseURL+"/confirm/"+token, http.NoBody)
	require.NoError(t, err, "build confirm request")

	resp, err := client.Do(req)
	require.NoError(t, err, "do confirm request")
	defer func() {
		_ = resp.Body.Close()
	}()

	require.Equal(t, expectedCode, resp.StatusCode, "unexpected confirm status")
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

func getConfirmTokenByEmail(t *testing.T, databaseURL, email string) string {
	t.Helper()

	db, err := sql.Open("pgx", databaseURL)
	require.NoError(t, err, "open db")
	defer func() {
		_ = db.Close()
	}()

	ctx := t.Context()

	var token string
	err = db.QueryRowContext(ctx, "SELECT confirm_token FROM subscriptions WHERE email=$1", email).Scan(&token)
	require.NoError(t, err, "get confirm token")

	return token
}
