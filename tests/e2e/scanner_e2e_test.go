//go:build e2e

package e2e

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/brianvoe/gofakeit/v7"
	gh "github.com/google/go-github/v84/github"
	"github.com/stretchr/testify/require"
)

const scannerE2ERepository = "dmytrovoron/github-release-notification-e2e-test"

func TestScannerE2E_ReleaseTriggersNotificationEmail(t *testing.T) {
	e := setupScanner(t)

	email := gofakeit.Email()
	err := e.subscribe(t, email, scannerE2ERepository)
	require.NoError(t, err, "subscribe should succeed")

	e.activateSubscriptionByEmail(t, email)
	baselineTag := e.waitForRepositoryStateTag(t, scannerE2ERepository, 45*time.Second)
	require.NotEmpty(t, baselineTag, "scanner should initialize repository state before release is created")

	releaseTag := fmt.Sprintf("e2e-scanner-%d", time.Now().UnixNano())
	releaseID := e.createGitHubRelease(t, scannerE2ERepository, releaseTag)
	t.Cleanup(func() { e.cleanupDeleteGitHubRelease(t, scannerE2ERepository, releaseID, releaseTag) })

	e.waitForMailpitReleaseEmail(t, email, scannerE2ERepository, releaseTag, 90*time.Second)
}

func (e *e2eScanner) requireRepositoryAccess(t *testing.T, repositoryName string) {
	t.Helper()

	owner, repo, ok := strings.Cut(repositoryName, "/")
	require.True(t, ok, "repository name must be owner/repo")

	_, resp, err := e.gh.Repositories.Get(t.Context(), owner, repo)
	if err == nil {
		return
	}

	if ghErr, ok := errors.AsType[*gh.ErrorResponse](err); ok {
		switch ghErr.Response.StatusCode {
		case http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound:
			t.Skipf(
				"GITHUB_AUTH_TOKEN must have read access to %s (github status %d)",
				repositoryName,
				ghErr.Response.StatusCode,
			)
		default:
		}
	}

	if resp != nil {
		t.Fatalf("verify repository access for %s: %v (status %d)", repositoryName, err, resp.StatusCode)
	}
	t.Fatalf("verify repository access for %s: %v", repositoryName, err)
}

func (e *e2eScanner) createGitHubRelease(t *testing.T, repositoryName, tag string) int64 {
	t.Helper()

	owner, repo, ok := strings.Cut(repositoryName, "/")
	require.True(t, ok, "repository name must be owner/repo")

	created, _, err := e.gh.Repositories.CreateRelease(t.Context(), owner, repo, &gh.RepositoryRelease{
		TagName: &tag,
		Name:    &tag,
		Body:    new("e2e scanner release"),
	})
	if ghErr, ok := errors.AsType[*gh.ErrorResponse](err); ok {
		switch ghErr.Response.StatusCode {
		case http.StatusUnauthorized, http.StatusForbidden:
			t.Skipf(
				"GITHUB_AUTH_TOKEN must allow creating releases for %s (github status %d)",
				repositoryName,
				ghErr.Response.StatusCode,
			)
		}
	}
	require.NoError(t, err, "create github release")
	require.NotNil(t, created)
	require.NotZero(t, created.GetID())

	return created.GetID()
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

func (e *e2e) waitForRepositoryStateTag(t *testing.T, repositoryName string, timeout time.Duration) string {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		var tag string
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
		found := e.findMailpitReleaseEmail(t, recipient, repositoryName, releaseTag)
		if found {
			return
		}

		time.Sleep(2 * time.Second)
	}

	t.Fatalf("timed out waiting for release email to %s for %s tag %s", recipient, repositoryName, releaseTag)
}

func (e *e2e) findMailpitReleaseEmail(t *testing.T, recipient, repositoryName, releaseTag string) bool {
	t.Helper()

	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, e.smtpAPIBaseURL+"/api/v1/messages", http.NoBody)
	require.NoError(t, err, "request mailpit messages")

	client := &http.Client{}
	resp, err := client.Do(req)
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
