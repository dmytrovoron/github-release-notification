//go:build e2e

package e2e_test

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/brianvoe/gofakeit/v7"
	"github.com/google/go-github/v84/github"
	"github.com/stretchr/testify/require"

	subclient "github.com/dmytrovoron/github-release-notification/tests/http/client"
	"github.com/dmytrovoron/github-release-notification/tests/http/client/subscription"
)

const scannerE2ERepository = "dmytrovoron/github-release-notification-e2e-test"

var eScanner e2eScanner

func TestScannerE2E(t *testing.T) {
	t.Parallel()

	t.Run("repository tag changed updates repository state in database", func(t *testing.T) {
		email := gofakeit.Email()

		releaseTagInitial := fmt.Sprintf("e2e-scanner-%d", time.Now().UnixNano())
		eScanner.createGitHubRelease(t, scannerE2ERepository, releaseTagInitial)

		err := eScanner.subscribe(t, email, scannerE2ERepository)
		require.NoError(t, err, "subscribe should succeed")

		eScanner.activateSubscriptionByEmail(t, email)
		baselineTag := eScanner.waitForRepositoryStateTag(t, scannerE2ERepository, 30*time.Second)
		require.NotEmpty(t, baselineTag, "scanner should initialize repository state before release is created")

		releaseTag := fmt.Sprintf("e2e-scanner-%d", time.Now().UnixNano())
		eScanner.createGitHubRelease(t, scannerE2ERepository, releaseTag)

		eScanner.waitForRepositoryStateTagEqual(t, scannerE2ERepository, releaseTag, 30*time.Second)
	})
}

type e2eScanner struct {
	client *subclient.GitHubReleaseNotificationAPI
	db     *sql.DB
	gh     *github.Client
}

func (e *e2eScanner) subscribe(t *testing.T, email, repo string) error {
	t.Helper()

	params := subscription.NewSubscribeParamsWithContext(t.Context()).WithEmail(email).WithRepo(repo)
	_, err := e.client.Subscription.Subscribe(params, subscription.WithContentType("application/x-www-form-urlencoded"))

	return err
}

func (e *e2eScanner) createGitHubRelease(t *testing.T, repositoryName, tag string) {
	t.Helper()

	owner, repo, ok := strings.Cut(repositoryName, "/")
	require.True(t, ok, "repository name must be owner/repo")

	created, _, err := e.gh.Repositories.CreateRelease(t.Context(), owner, repo, &github.RepositoryRelease{
		TagName: &tag,
		Name:    &tag,
		Body:    new("e2e scanner release"),
	})
	require.NoError(t, err, "create github release")
	require.NotZero(t, created.GetID())

	t.Cleanup(func() { e.cleanupDeleteGitHubRelease(t, scannerE2ERepository, created.GetID(), tag) })
}

func (e *e2eScanner) cleanupDeleteGitHubRelease(t *testing.T, repositoryName string, releaseID int64, tag string) {
	t.Helper()

	//nolint:usetesting // context.Background is used intentionally instead of t.Context
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	owner, repo, ok := strings.Cut(repositoryName, "/")
	if !ok {
		return
	}

	_, err := e.gh.Repositories.DeleteRelease(ctx, owner, repo, releaseID)
	require.NoError(t, err, "delete github release")

	_, err = e.gh.Git.DeleteRef(ctx, owner, repo, "refs/tags/"+tag)
	require.NoError(t, err, "delete github tag ref")
}

func (e *e2eScanner) waitForRepositoryStateTagEqual(t *testing.T, repositoryName, expectedTag string, timeout time.Duration) {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		var tag string
		//nolint:unqueryvet // it's ok in tests
		err := e.db.QueryRowContext(
			t.Context(),
			"SELECT last_seen_tag FROM repository_states WHERE repository=$1",
			repositoryName,
		).Scan(&tag)
		if err == nil && tag == expectedTag {
			return
		}

		time.Sleep(1500 * time.Millisecond)
	}

	t.Fatalf("timed out waiting for repository_states.last_seen_tag=%s for %s", expectedTag, repositoryName)
}

func (e *e2eScanner) waitForRepositoryStateTag(t *testing.T, repositoryName string, timeout time.Duration) string {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		var tag string
		//nolint:unqueryvet // it's ok in tests
		err := e.db.QueryRowContext(
			t.Context(),
			"SELECT last_seen_tag FROM repository_states WHERE repository=$1",
			repositoryName,
		).Scan(&tag)
		if err == nil && strings.TrimSpace(tag) != "" {
			return tag
		}

		time.Sleep(1500 * time.Millisecond)
	}

	t.Fatalf("timed out waiting for repository_states.last_seen_tag for %s", repositoryName)

	return ""
}

func (e *e2eScanner) activateSubscriptionByEmail(t *testing.T, email string) {
	t.Helper()

	_, err := e.db.ExecContext(t.Context(), "UPDATE subscriptions SET status='active' WHERE email=$1", email)
	require.NoError(t, err, "activate subscription")
}
