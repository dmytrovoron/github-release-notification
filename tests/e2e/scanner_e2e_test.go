//go:build e2e

package e2e

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/brianvoe/gofakeit/v7"
	gh "github.com/google/go-github/v84/github"
	"github.com/stretchr/testify/require"
)

const scannerE2ERepository = "dmytrovoron/github-release-notification-e2e-test"

func TestScannerE2E(t *testing.T) {
	t.Parallel()

	t.Run("repository tag changed updates repository state in database", func(t *testing.T) {
		email := gofakeit.Email()

		releaseTagInitial := fmt.Sprintf("e2e-scanner-%d", time.Now().UnixNano())
		e.createGitHubRelease(t, scannerE2ERepository, releaseTagInitial)

		err := e.subscribe(t, email, scannerE2ERepository)
		require.NoError(t, err, "subscribe should succeed")

		e.activateSubscriptionByEmail(t, email)
		baselineTag := e.waitForRepositoryStateTag(t, scannerE2ERepository, 30*time.Second)
		require.NotEmpty(t, baselineTag, "scanner should initialize repository state before release is created")

		releaseTag := fmt.Sprintf("e2e-scanner-%d", time.Now().UnixNano())
		e.createGitHubRelease(t, scannerE2ERepository, releaseTag)

		e.waitForRepositoryStateTagEqual(t, scannerE2ERepository, releaseTag, 30*time.Second)
	})
}

func (e *e2e) requireRepositoryAccess(ctx context.Context, repositoryName string) error {
	owner, repo, ok := strings.Cut(repositoryName, "/")
	if !ok {
		return fmt.Errorf("repository name must be owner/repo, got %q", repositoryName)
	}

	_, resp, err := e.gh.Repositories.Get(ctx, owner, repo)
	if err == nil {
		return nil
	}

	if ghErr, ok := errors.AsType[*gh.ErrorResponse](err); ok {
		switch ghErr.Response.StatusCode {
		case http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound:
			return fmt.Errorf(
				"GITHUB_AUTH_TOKEN must have read and write access to %s (github status %d)",
				repositoryName,
				ghErr.Response.StatusCode,
			)
		default:
		}
	}

	if resp != nil {
		return fmt.Errorf("verify repository access for %s: %w (status %d)", repositoryName, err, resp.StatusCode)
	}

	return fmt.Errorf("verify repository access for %s: %w", repositoryName, err)
}

func (e *e2e) createGitHubRelease(t *testing.T, repositoryName, tag string) {
	t.Helper()

	owner, repo, ok := strings.Cut(repositoryName, "/")
	require.True(t, ok, "repository name must be owner/repo")

	created, _, err := e.gh.Repositories.CreateRelease(t.Context(), owner, repo, &gh.RepositoryRelease{
		TagName: &tag,
		Name:    &tag,
		Body:    new("e2e scanner release"),
	})
	require.NoError(t, err, "create github release")
	require.NotZero(t, created.GetID())

	t.Cleanup(func() { e.cleanupDeleteGitHubRelease(t, scannerE2ERepository, created.GetID(), tag) })
}

func (e *e2e) cleanupDeleteGitHubRelease(t *testing.T, repositoryName string, releaseID int64, tag string) {
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

func (e *e2e) waitForRepositoryStateTag(t *testing.T, repositoryName string, timeout time.Duration) string {
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

func (e *e2e) waitForRepositoryStateTagEqual(t *testing.T, repositoryName, expectedTag string, timeout time.Duration) {
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
