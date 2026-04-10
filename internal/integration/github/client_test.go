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

	// use real GitHub API in integration test
	authToken := os.Getenv("GITHUB_AUTH_TOKEN")
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
			assert.Equal(t, tc.want, got)
		})
	}
}
