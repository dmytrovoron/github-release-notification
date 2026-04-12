//go:build integration

package github_test

import (
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dmytrovoron/github-release-notification/internal/integration/github"
)

func TestClient_RepositoryExists(t *testing.T) {
	t.Parallel()

	authToken := skipIfNoAuthToken(t)

	client := github.NewClient(authToken, 10*time.Second)

	tests := []struct {
		name  string
		owner string
		repo  string
		want  bool
	}{
		{
			name:  "existing repository",
			owner: "octocat",
			repo:  "Hello-World",
			want:  true,
		},
		{
			name:  "non-existing repository",
			owner: "octocat",
			repo:  "non-existing-repository-for-test",
			want:  false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := client.RepositoryExists(t.Context(), tc.owner, tc.repo)
			require.NoError(t, err)
			assert.Equalf(t, tc.want, got, "RepositoryExists(%q, %q) = %v, want %v", tc.owner, tc.repo, got, tc.want)
		})
	}
}

func TestClient_LatestReleaseTag(t *testing.T) {
	t.Parallel()

	authToken := skipIfNoAuthToken(t)

	client := github.NewClient(authToken, 10*time.Second)

	tests := []struct {
		name  string
		owner string
		repo  string
		empty bool
	}{
		{
			name:  "repository with releases",
			owner: "golangci",
			repo:  "golangci-lint",
			empty: false,
		},
		{
			name:  "repository without releases",
			owner: "octocat",
			repo:  "Hello-World",
			empty: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			tag, err := client.LatestReleaseTag(t.Context(), tc.owner, tc.repo)
			require.NoError(t, err)
			if tc.empty {
				assert.Emptyf(t, tag, "LatestReleaseTag(%q, %q) = %q, want empty", tc.owner, tc.repo, tag)

				return
			}

			assert.NotEmptyf(t, tag, "LatestReleaseTag(%q, %q) = %q, want non-empty", tc.owner, tc.repo, tag)
		})
	}
}

func skipIfNoAuthToken(t *testing.T) string {
	t.Helper()

	authToken := os.Getenv("GITHUB_AUTH_TOKEN")
	if authToken == "" {
		t.Skip("Set GITHUB_AUTH_TOKEN environment variable to run this test")
	}

	return authToken
}
