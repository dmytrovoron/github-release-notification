//go:build e2e

package e2e_test

import (
	"database/sql"
	"math/rand/v2"
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

var eAPI e2eAPI

func TestAPI_SubscribeEndpointE2E(t *testing.T) {
	t.Parallel()

	t.Run("Subscription successful", func(t *testing.T) {
		t.Parallel()

		email := gofakeit.Email()
		repo := randRealRepo()

		err := eAPI.subscribe(t, email, repo)

		require.NoError(t, err, "subscribe should succeed")
	})

	t.Run("Invalid input: email", func(t *testing.T) {
		t.Parallel()

		err := eAPI.subscribe(t, "invalid-email", randRealRepo())

		var badRequest *subscription.SubscribeBadRequest
		require.ErrorAs(t, err, &badRequest)
	})

	t.Run("Invalid input: repo", func(t *testing.T) {
		t.Parallel()

		err := eAPI.subscribe(t, gofakeit.Email(), "invalid/repo/name/with/slashes")

		var badRequest *subscription.SubscribeBadRequest
		require.ErrorAs(t, err, &badRequest)
	})

	t.Run("Repository not found on GitHub", func(t *testing.T) {
		t.Parallel()

		email := gofakeit.Email()

		err := eAPI.subscribe(t, email, "non-existing/repo-for-test")

		var notFound *subscription.SubscribeNotFound
		require.ErrorAs(t, err, &notFound)
	})

	t.Run("Email already subscribed to this repository", func(t *testing.T) {
		t.Parallel()

		email := gofakeit.Email()
		repo := randRealRepo()

		err := eAPI.subscribe(t, email, repo)
		require.NoError(t, err, "initial subscribe should succeed")

		err = eAPI.subscribe(t, email, repo)
		var conflict *subscription.SubscribeConflict
		require.ErrorAs(t, err, &conflict)
	})
}

func TestAPI_ConfirmEndpointE2E(t *testing.T) {
	t.Parallel()

	t.Run("Confirm subscription successful", func(t *testing.T) {
		t.Parallel()

		email := gofakeit.Email()
		repo := randRealRepo()

		err := eAPI.subscribe(t, email, repo)
		require.NoError(t, err, "initial subscribe should succeed")

		token := eAPI.getConfirmTokenByEmail(t, email)

		err = eAPI.confirm(t, token)
		require.NoError(t, err, "confirm should succeed")

		expectedItems := []*models.Subscription{
			{
				Email:     &email,
				Repo:      &repo,
				Confirmed: true,
			},
		}
		actualItems, err := eAPI.getSubscriptions(t, email)
		require.NoError(t, err, "get subscriptions should succeed")
		assert.Equal(t, expectedItems, actualItems)
	})

	t.Run("Invalid token", func(t *testing.T) {
		t.Parallel()

		err := eAPI.confirm(t, "abcd")

		var badRequest *subscription.ConfirmSubscriptionBadRequest
		require.ErrorAs(t, err, &badRequest)
	})

	t.Run("Token not found", func(t *testing.T) {
		t.Parallel()

		err := eAPI.confirm(t, "0123456789abcdef")

		var notFound *subscription.ConfirmSubscriptionNotFound
		require.ErrorAs(t, err, &notFound)
	})
}

func TestAPI_GetSubscriptionsEndpointE2E(t *testing.T) {
	t.Parallel()

	t.Run("Successful operation - list of subscriptions returned", func(t *testing.T) {
		t.Parallel()

		email := gofakeit.Email()
		repo := randRealRepo()

		err := eAPI.subscribe(t, email, repo)
		require.NoError(t, err, "initial subscribe should succeed")
		eAPI.activateSubscriptionByEmail(t, email)

		expectedItems := []*models.Subscription{
			{
				Email:     &email,
				Repo:      &repo,
				Confirmed: true,
			},
		}

		actualItems, err := eAPI.getSubscriptions(t, email)
		require.NoError(t, err, "get subscriptions should succeed")
		assert.Equal(t, expectedItems, actualItems)
	})

	t.Run("Invalid email", func(t *testing.T) {
		t.Parallel()

		_, err := eAPI.getSubscriptions(t, "invalid-email")

		var badRequest *subscription.GetSubscriptionsBadRequest
		require.ErrorAs(t, err, &badRequest)
	})
}

func TestAPI_UnsubscribeEndpointE2E(t *testing.T) {
	t.Parallel()

	t.Run("Unsubscribed successful", func(t *testing.T) {
		t.Parallel()

		email := gofakeit.Email()
		repo := randRealRepo()

		err := eAPI.subscribe(t, email, repo)
		require.NoError(t, err, "initial subscribe should succeed")
		eAPI.activateSubscriptionByEmail(t, email)

		token := eAPI.getUnsubscribeTokenByEmail(t, email)
		err = eAPI.unsubscribe(t, token)
		require.NoError(t, err, "unsubscribe should succeed")

		actualItems, err := eAPI.getSubscriptions(t, email)
		require.NoError(t, err, "get subscriptions should succeed")
		assert.Empty(t, actualItems)
	})

	t.Run("Invalid token", func(t *testing.T) {
		t.Parallel()

		err := eAPI.unsubscribe(t, "abcd")

		var badRequest *subscription.UnsubscribeBadRequest
		require.ErrorAs(t, err, &badRequest)
	})

	t.Run("Token not found", func(t *testing.T) {
		t.Parallel()

		err := eAPI.unsubscribe(t, "fedcba9876543210")

		var notFound *subscription.UnsubscribeNotFound
		require.ErrorAs(t, err, &notFound)
	})
}

type e2eAPI struct {
	client *subclient.GitHubReleaseNotificationAPI
	db     *sql.DB
}

func (e *e2eAPI) subscribe(t *testing.T, email, repo string) error {
	t.Helper()

	params := subscription.NewSubscribeParamsWithContext(t.Context()).WithEmail(email).WithRepo(repo)
	_, err := e.client.Subscription.Subscribe(params, subscription.WithContentType("application/x-www-form-urlencoded"))

	return err
}

func (e *e2eAPI) confirm(t *testing.T, token string) error {
	t.Helper()

	params := subscription.NewConfirmSubscriptionParamsWithContext(t.Context()).WithToken(token)
	_, err := e.client.Subscription.ConfirmSubscription(params)

	return err
}

func (e *e2eAPI) unsubscribe(t *testing.T, token string) error {
	t.Helper()

	params := subscription.NewUnsubscribeParamsWithContext(t.Context()).WithToken(token)
	_, err := e.client.Subscription.Unsubscribe(params)

	return err
}

func (e *e2eAPI) getSubscriptions(t *testing.T, email string) ([]*models.Subscription, error) {
	t.Helper()

	params := subscription.NewGetSubscriptionsParamsWithContext(t.Context()).WithEmail(email)
	result, err := e.client.Subscription.GetSubscriptions(params)
	if err != nil {
		return nil, err
	}

	return result.Payload, nil
}

func (e *e2eAPI) activateSubscriptionByEmail(t *testing.T, email string) {
	t.Helper()

	_, err := e.db.ExecContext(t.Context(), "UPDATE subscriptions SET status='active' WHERE email=$1", email)
	require.NoError(t, err, "activate subscription")
}

func (e *e2eAPI) getConfirmTokenByEmail(t *testing.T, email string) string {
	t.Helper()

	var token string
	err := e.db.QueryRowContext(t.Context(), "SELECT confirm_token FROM subscriptions WHERE email=$1", email).Scan(&token)
	require.NoError(t, err, "get confirm token")

	return token
}

func (e *e2eAPI) getUnsubscribeTokenByEmail(t *testing.T, email string) string {
	t.Helper()

	var token string
	err := e.db.QueryRowContext(t.Context(), "SELECT unsubscribe_token FROM subscriptions WHERE email=$1", email).Scan(&token)
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
