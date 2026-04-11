//go:build e2e

package e2e

import (
	"database/sql"
	"math/rand/v2"
	"net/url"
	"testing"

	"github.com/brianvoe/gofakeit/v7"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	subclient "github.com/dmytrovoron/github-release-notification/tests/http/client"
	"github.com/dmytrovoron/github-release-notification/tests/http/client/subscription"
	"github.com/dmytrovoron/github-release-notification/tests/http/models"
)

//go:generate go tool -modfile=../../tools/go.mod github.com/go-swagger/go-swagger/cmd/swagger generate client --spec ../../api/swagger.yaml --target ../http

type e2eEnv struct {
	baseURL            string
	databaseURLForTest string
}

func TestSubscribeEndpointE2E(t *testing.T) {
	env := setupE2EEnv(t)
	api := newTestAPIClient(t, env.baseURL)

	t.Run("Subscription successful", func(t *testing.T) {
		email := gofakeit.Email()
		repo := randRealRepo()

		subscribe(t, api, email, repo)

		activateSubscriptionByEmail(t, env.databaseURLForTest, email)

		expectedItems := []*models.Subscription{
			{
				Email:     &email,
				Repo:      &repo,
				Confirmed: true,
			},
		}
		actualItems := getSubscriptions(t, api, email)
		assert.Equal(t, expectedItems, actualItems)
	})

	t.Run("Invalid input", func(t *testing.T) {
		subscribeExpectBadRequest(t, api, "invalid-email", randRealRepo())
		subscribeExpectBadRequest(t, api, gofakeit.Email(), "invalid/repo/name/with/slashes")
	})

	t.Run("Repository not found on GitHub", func(t *testing.T) {
		email := gofakeit.Email()

		subscribeExpectNotFound(t, api, email, "non-existing/repo-for-test")
	})

	t.Run("Email already subscribed to this repository", func(t *testing.T) {
		email := gofakeit.Email()
		repo := randRealRepo()

		subscribe(t, api, email, repo)
		subscribeExpectConflict(t, api, email, repo)
	})
}

func TestConfirmEndpointE2E(t *testing.T) {
	env := setupE2EEnv(t)
	api := newTestAPIClient(t, env.baseURL)

	t.Run("Confirm subscription successful", func(t *testing.T) {
		email := gofakeit.Email()
		repo := randRealRepo()

		subscribe(t, api, email, repo)

		token := getConfirmTokenByEmail(t, env.databaseURLForTest, email)

		confirm(t, api, token)

		expectedItems := []*models.Subscription{
			{
				Email:     &email,
				Repo:      &repo,
				Confirmed: true,
			},
		}
		actualItems := getSubscriptions(t, api, email)
		assert.Equal(t, expectedItems, actualItems)
	})

	t.Run("Token not found", func(t *testing.T) {
		confirmExpectNotFound(t, api, "nonexistenttoken123")
	})

	t.Run("Empty token", func(t *testing.T) {
		confirmExpectBadRequest(t, api, " ")
	})
}

func TestGetSubscriptionsEndpointE2E(t *testing.T) {
	env := setupE2EEnv(t)
	api := newTestAPIClient(t, env.baseURL)

	t.Run("Get subscriptions successful", func(t *testing.T) {
		email := gofakeit.Email()
		repo := randRealRepo()

		subscribe(t, api, email, repo)
		activateSubscriptionByEmail(t, env.databaseURLForTest, email)

		expectedItems := []*models.Subscription{
			{
				Email:     &email,
				Repo:      &repo,
				Confirmed: true,
			},
		}

		actualItems := getSubscriptions(t, api, email)
		assert.Equal(t, expectedItems, actualItems)
	})

	t.Run("Invalid email", func(t *testing.T) {
		getSubscriptionsExpectBadRequest(t, api, "invalid-email")
	})
}

func TestUnsubscribeEndpointE2E(t *testing.T) {
	env := setupE2EEnv(t)
	api := newTestAPIClient(t, env.baseURL)

	t.Run("Unsubscribe successful", func(t *testing.T) {
		email := gofakeit.Email()
		repo := randRealRepo()

		subscribe(t, api, email, repo)
		activateSubscriptionByEmail(t, env.databaseURLForTest, email)

		token := getUnsubscribeTokenByEmail(t, env.databaseURLForTest, email)
		unsubscribe(t, api, token)

		actualItems := getSubscriptions(t, api, email)
		assert.Empty(t, actualItems)
	})

	t.Run("Token not found", func(t *testing.T) {
		unsubscribeExpectNotFound(t, api, "nonexistenttoken123")
	})

	t.Run("Empty token", func(t *testing.T) {
		unsubscribeExpectBadRequest(t, api, " ")
	})
}

func newTestAPIClient(t *testing.T, baseURL string) *subclient.GitHubReleaseNotificationAPI {
	t.Helper()

	parsedURL, err := url.Parse(baseURL)
	require.NoError(t, err, "parse api base url")
	require.NotEmpty(t, parsedURL.Host, "api base url host must not be empty")

	basePath := parsedURL.Path
	if basePath == "" {
		basePath = "/"
	}

	cfg := subclient.DefaultTransportConfig().
		WithHost(parsedURL.Host).
		WithBasePath(basePath).
		WithSchemes([]string{parsedURL.Scheme})

	return subclient.NewHTTPClientWithConfig(nil, cfg)
}

func subscribe(t *testing.T, api *subclient.GitHubReleaseNotificationAPI, email, repo string) {
	t.Helper()

	params := subscription.NewSubscribeParamsWithContext(t.Context()).
		WithEmail(email).
		WithRepo(repo)
	_, err := api.Subscription.Subscribe(params, subscription.WithContentType("application/x-www-form-urlencoded"))
	require.NoError(t, err, "subscribe should succeed")
}

func subscribeExpectBadRequest(t *testing.T, api *subclient.GitHubReleaseNotificationAPI, email, repo string) {
	t.Helper()

	params := subscription.NewSubscribeParamsWithContext(t.Context()).
		WithEmail(email).
		WithRepo(repo)
	_, err := api.Subscription.Subscribe(params, subscription.WithContentType("application/x-www-form-urlencoded"))
	require.Error(t, err)
	var badRequest *subscription.SubscribeBadRequest
	require.ErrorAs(t, err, &badRequest)
}

func subscribeExpectNotFound(t *testing.T, api *subclient.GitHubReleaseNotificationAPI, email, repo string) {
	t.Helper()

	params := subscription.NewSubscribeParamsWithContext(t.Context()).
		WithEmail(email).
		WithRepo(repo)
	_, err := api.Subscription.Subscribe(params, subscription.WithContentType("application/x-www-form-urlencoded"))
	require.Error(t, err)
	var notFound *subscription.SubscribeNotFound
	require.ErrorAs(t, err, &notFound)
}

func subscribeExpectConflict(t *testing.T, api *subclient.GitHubReleaseNotificationAPI, email, repo string) {
	t.Helper()

	params := subscription.NewSubscribeParamsWithContext(t.Context()).
		WithEmail(email).
		WithRepo(repo)
	_, err := api.Subscription.Subscribe(params, subscription.WithContentType("application/x-www-form-urlencoded"))
	require.Error(t, err)
	var conflict *subscription.SubscribeConflict
	require.ErrorAs(t, err, &conflict)
}

func confirm(t *testing.T, api *subclient.GitHubReleaseNotificationAPI, token string) {
	t.Helper()

	params := subscription.NewConfirmSubscriptionParamsWithContext(t.Context()).WithToken(token)
	_, err := api.Subscription.ConfirmSubscription(params)
	require.NoError(t, err, "confirm should succeed")
}

func confirmExpectNotFound(t *testing.T, api *subclient.GitHubReleaseNotificationAPI, token string) {
	t.Helper()

	params := subscription.NewConfirmSubscriptionParamsWithContext(t.Context()).WithToken(token)
	_, err := api.Subscription.ConfirmSubscription(params)
	require.Error(t, err)
	var notFound *subscription.ConfirmSubscriptionNotFound
	require.ErrorAs(t, err, &notFound)
}

func confirmExpectBadRequest(t *testing.T, api *subclient.GitHubReleaseNotificationAPI, token string) {
	t.Helper()

	params := subscription.NewConfirmSubscriptionParamsWithContext(t.Context()).WithToken(token)
	_, err := api.Subscription.ConfirmSubscription(params)
	require.Error(t, err)
	var badRequest *subscription.ConfirmSubscriptionBadRequest
	require.ErrorAs(t, err, &badRequest)
}

func unsubscribe(t *testing.T, api *subclient.GitHubReleaseNotificationAPI, token string) {
	t.Helper()

	params := subscription.NewUnsubscribeParamsWithContext(t.Context()).WithToken(token)
	_, err := api.Subscription.Unsubscribe(params)
	require.NoError(t, err, "unsubscribe should succeed")
}

func unsubscribeExpectNotFound(t *testing.T, api *subclient.GitHubReleaseNotificationAPI, token string) {
	t.Helper()

	params := subscription.NewUnsubscribeParamsWithContext(t.Context()).WithToken(token)
	_, err := api.Subscription.Unsubscribe(params)
	require.Error(t, err)
	var notFound *subscription.UnsubscribeNotFound
	require.ErrorAs(t, err, &notFound)
}

func unsubscribeExpectBadRequest(t *testing.T, api *subclient.GitHubReleaseNotificationAPI, token string) {
	t.Helper()

	params := subscription.NewUnsubscribeParamsWithContext(t.Context()).WithToken(token)
	_, err := api.Subscription.Unsubscribe(params)
	require.Error(t, err)
	var badRequest *subscription.UnsubscribeBadRequest
	require.ErrorAs(t, err, &badRequest)
}

func getSubscriptions(t *testing.T, api *subclient.GitHubReleaseNotificationAPI, email string) []*models.Subscription {
	t.Helper()

	params := subscription.NewGetSubscriptionsParamsWithContext(t.Context()).WithEmail(email)
	result, err := api.Subscription.GetSubscriptions(params)
	require.NoError(t, err, "get subscriptions should succeed")

	return result.Payload
}

func getSubscriptionsExpectBadRequest(t *testing.T, api *subclient.GitHubReleaseNotificationAPI, email string) {
	t.Helper()

	params := subscription.NewGetSubscriptionsParamsWithContext(t.Context()).WithEmail(email)
	_, err := api.Subscription.GetSubscriptions(params)
	require.Error(t, err)
	var badRequest *subscription.GetSubscriptionsBadRequest
	require.ErrorAs(t, err, &badRequest)
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

func getUnsubscribeTokenByEmail(t *testing.T, databaseURL, email string) string {
	t.Helper()

	db, err := sql.Open("pgx", databaseURL)
	require.NoError(t, err, "open db")
	defer func() {
		_ = db.Close()
	}()

	ctx := t.Context()

	var token string
	err = db.QueryRowContext(ctx, "SELECT unsubscribe_token FROM subscriptions WHERE email=$1", email).Scan(&token)
	require.NoError(t, err, "get unsubscribe token")

	return token
}

// randRealRepo returns the name of a random popular public repository on GitHub for testing.
func randRealRepo() string {
	repos := []string{
		"facebook/react",
		"golang/go",
		"microsoft/vscode",
		"octocat/Hello-World",
		"tensorflow/tensorflow",
		"torvalds/linux",
	}

	return repos[rand.N(len(repos))]
}
