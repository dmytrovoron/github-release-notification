package scanner

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	app "github.com/dmytrovoron/github-release-notification/internal"
	"github.com/dmytrovoron/github-release-notification/internal/repository"
)

type fakeGitHubClient struct {
	tags map[string]string
}

func (f *fakeGitHubClient) LatestReleaseTag(_ context.Context, owner, repo string) (string, error) {
	return f.tags[owner+"/"+repo], nil
}

type fakeScannerRepository struct {
	subscriptions []repository.Subscription
	results       map[string]repository.RepositoryTagUpdateResult
	advanced      []string
}

func (f *fakeScannerRepository) ListActive(_ context.Context) ([]repository.Subscription, error) {
	return f.subscriptions, nil
}

func (f *fakeScannerRepository) AdvanceRepositoryTag(
	_ context.Context,
	repositoryName string,
	_ string,
) (repository.RepositoryTagUpdateResult, error) {
	f.advanced = append(f.advanced, repositoryName)
	if result, ok := f.results[repositoryName]; ok {
		return result, nil
	}

	return repository.RepositoryTagUnchanged, nil
}

func TestRunner_RunOnce_RepositoryTagChanged_AdvancesTag(t *testing.T) {
	t.Parallel()

	repo := &fakeScannerRepository{
		subscriptions: []repository.Subscription{
			{
				ID:         1,
				Email:      "a@example.com",
				Repository: "golang/go",
				Status:     app.SubscriptionStatusActive,
			},
			{
				ID:         2,
				Email:      "b@example.com",
				Repository: "golang/go",
				Status:     app.SubscriptionStatusActive,
			},
		},
		results: map[string]repository.RepositoryTagUpdateResult{
			"golang/go": repository.RepositoryTagChanged,
		},
	}
	githubClient := &fakeGitHubClient{tags: map[string]string{"golang/go": "v1.2.3"}}

	runner := NewRunner(
		slog.New(slog.DiscardHandler),
		repo,
		githubClient,
		time.Second,
	)

	runner.RunOnce(t.Context())

	assert.Equal(t, []string{"golang/go"}, repo.advanced)
}

func TestRunner_RunOnce_InitializedOrUnchanged_AdvancesTag(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		result repository.RepositoryTagUpdateResult
	}{
		{
			name:   "initialized state",
			result: repository.RepositoryTagInitialized,
		},
		{
			name:   "unchanged state",
			result: repository.RepositoryTagUnchanged,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			repo := &fakeScannerRepository{
				subscriptions: []repository.Subscription{
					{
						ID:         1,
						Email:      "a@example.com",
						Repository: "golang/go",
						Status:     app.SubscriptionStatusActive,
					},
				},
				results: map[string]repository.RepositoryTagUpdateResult{
					"golang/go": tc.result,
				},
			}
			githubClient := &fakeGitHubClient{tags: map[string]string{"golang/go": "v1.2.3"}}

			runner := NewRunner(
				slog.New(slog.DiscardHandler),
				repo,
				githubClient,
				time.Second,
			)

			runner.RunOnce(t.Context())

			assert.Equal(t, []string{"golang/go"}, repo.advanced)
		})
	}
}
