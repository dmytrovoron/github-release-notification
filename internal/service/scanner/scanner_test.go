package scanner_test

import (
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	app "github.com/dmytrovoron/github-release-notification/internal"
	"github.com/dmytrovoron/github-release-notification/internal/repository"
	service "github.com/dmytrovoron/github-release-notification/internal/service/scanner"
)

func TestRunner_RunOnce_MissingBranches_TableDriven(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name              string
		repo              *fakeScannerRepository
		github            *fakeGitHubClient
		wantGitHubCalls   []string
		wantAdvancedRepos []string
	}{
		{
			name:              "empty active subscriptions exits early",
			repo:              &fakeScannerRepository{subscriptions: []repository.Subscription{}},
			github:            &fakeGitHubClient{tags: map[string]string{}},
			wantGitHubCalls:   []string{},
			wantAdvancedRepos: []string{},
		},
		{
			name: "invalid repository name is skipped",
			repo: &fakeScannerRepository{subscriptions: []repository.Subscription{
				{ID: 1, Email: "a@example.com", Repository: "invalid", Status: app.SubscriptionStatusActive},
			}},
			github:            &fakeGitHubClient{tags: map[string]string{}},
			wantGitHubCalls:   []string{},
			wantAdvancedRepos: []string{},
		},
		{
			name: "github fetch failure continues without advance",
			repo: &fakeScannerRepository{subscriptions: []repository.Subscription{
				{ID: 1, Email: "a@example.com", Repository: "golang/go", Status: app.SubscriptionStatusActive},
			}},
			github: &fakeGitHubClient{
				tags:            map[string]string{"golang/go": "v1.2.3"},
				errByRepository: map[string]error{"golang/go": errors.New("github unavailable")},
			},
			wantGitHubCalls:   []string{"golang/go"},
			wantAdvancedRepos: []string{},
		},
		{
			name: "advance failure still attempts advance once",
			repo: &fakeScannerRepository{
				subscriptions: []repository.Subscription{
					{ID: 1, Email: "a@example.com", Repository: "golang/go", Status: app.SubscriptionStatusActive},
				},
				advanceErrByRepository: map[string]error{"golang/go": errors.New("db failure")},
			},
			github:            &fakeGitHubClient{tags: map[string]string{"golang/go": "v1.2.3"}},
			wantGitHubCalls:   []string{"golang/go"},
			wantAdvancedRepos: []string{"golang/go"},
		},
		{
			name: "duplicate subscribers call github once per repository",
			repo: &fakeScannerRepository{
				subscriptions: []repository.Subscription{
					{ID: 1, Email: "a@example.com", Repository: "golang/go", Status: app.SubscriptionStatusActive},
					{ID: 2, Email: "b@example.com", Repository: "golang/go", Status: app.SubscriptionStatusActive},
				},
				results: map[string]repository.RepositoryTagUpdateResult{"golang/go": repository.RepositoryTagChanged},
			},
			github:            &fakeGitHubClient{tags: map[string]string{"golang/go": "v1.2.3"}},
			wantGitHubCalls:   []string{"golang/go"},
			wantAdvancedRepos: []string{"golang/go"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			runner := service.NewRunner(
				slog.New(slog.DiscardHandler),
				tc.repo,
				tc.github,
				time.Second,
			)

			runner.RunOnce(t.Context())

			if len(tc.wantGitHubCalls) == 0 {
				assert.Empty(t, tc.github.called)
			} else {
				assert.Equal(t, tc.wantGitHubCalls, tc.github.called)
			}

			if len(tc.wantAdvancedRepos) == 0 {
				assert.Empty(t, tc.repo.advanced)
			} else {
				assert.Equal(t, tc.wantAdvancedRepos, tc.repo.advanced)
			}
		})
	}
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

	runner := service.NewRunner(
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

			runner := service.NewRunner(
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
