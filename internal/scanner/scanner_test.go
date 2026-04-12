package scanner

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	app "github.com/dmytrovoron/github-release-notification/internal"
	"github.com/dmytrovoron/github-release-notification/internal/notifier"
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

type fakeReleaseSender struct {
	emails []notifier.ReleaseEmail
}

func (f *fakeReleaseSender) SendRelease(_ context.Context, email notifier.ReleaseEmail) error {
	f.emails = append(f.emails, email)

	return nil
}

func TestRunner_RunOnce_RepositoryTagChanged_SendsNotifications(t *testing.T) {
	t.Parallel()

	repo := &fakeScannerRepository{
		subscriptions: []repository.Subscription{
			{
				ID:               1,
				Email:            "a@example.com",
				Repository:       "golang/go",
				Status:           app.SubscriptionStatusActive,
				UnsubscribeToken: "aaaabbbbccccdddd",
			},
			{
				ID:               2,
				Email:            "b@example.com",
				Repository:       "golang/go",
				Status:           app.SubscriptionStatusActive,
				UnsubscribeToken: "1111222233334444",
			},
		},
		results: map[string]repository.RepositoryTagUpdateResult{
			"golang/go": repository.RepositoryTagChanged,
		},
	}
	githubClient := &fakeGitHubClient{tags: map[string]string{"golang/go": "v1.2.3"}}
	sender := &fakeReleaseSender{}

	runner := NewRunner(
		slog.New(slog.DiscardHandler),
		repo,
		githubClient,
		sender,
		time.Second,
		"http://localhost:8080/api/unsubscribe",
	)

	runner.RunOnce(t.Context())

	require.Len(t, sender.emails, 2)
	assert.Equal(t, "a@example.com", sender.emails[0].Email)
	assert.Equal(t, "v1.2.3", sender.emails[0].Tag)
	assert.Equal(t, "http://localhost:8080/api/unsubscribe/aaaabbbbccccdddd", sender.emails[0].UnsubscribeURL)
	assert.Equal(t, "b@example.com", sender.emails[1].Email)
}

func TestRunner_RunOnce_InitializedOrUnchanged_DoesNotSendNotifications(t *testing.T) {
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
						ID:               1,
						Email:            "a@example.com",
						Repository:       "golang/go",
						Status:           app.SubscriptionStatusActive,
						UnsubscribeToken: "aaaabbbbccccdddd",
					},
				},
				results: map[string]repository.RepositoryTagUpdateResult{
					"golang/go": tc.result,
				},
			}
			githubClient := &fakeGitHubClient{tags: map[string]string{"golang/go": "v1.2.3"}}
			sender := &fakeReleaseSender{}

			runner := NewRunner(
				slog.New(slog.DiscardHandler),
				repo,
				githubClient,
				sender,
				time.Second,
				"http://localhost:8080/api/unsubscribe",
			)

			runner.RunOnce(t.Context())

			assert.Empty(t, sender.emails)
		})
	}
}
