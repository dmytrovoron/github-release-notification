package notifier_test

import (
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dmytrovoron/github-release-notification/internal/repository"
	"github.com/dmytrovoron/github-release-notification/internal/service/notifier"
)

func TestRunner_RunOnce_TableDriven(t *testing.T) {
	t.Parallel()

	pending := []repository.PendingNotification{
		{SubscriptionID: 1, Email: "a@example.com", Repository: "owner/repo-a", CurrentTag: "v1.0.0", UnsubscribeToken: "token-a"},
		{SubscriptionID: 2, Email: "b@example.com", Repository: "owner/repo-b", CurrentTag: "v2.0.0", UnsubscribeToken: "token-b"},
		{SubscriptionID: 3, Email: "c@example.com", Repository: "owner/repo-c", CurrentTag: "v3.0.0", UnsubscribeToken: "token-c"},
	}

	tests := []struct {
		name              string
		repo              *fakeNotifierRepository
		sender            *fakeReleaseSender
		unsubscribeBase   string
		wantSentCount     int
		wantMarkedCount   int
		wantMarkedSubIDs  []int64
		wantFirstURL      string
		wantFirstEmail    string
		wantNoSendOnError bool
	}{
		{
			name: "list pending error stops processing",
			repo: &fakeNotifierRepository{
				listErr: errors.New("db is down"),
			},
			sender:            &fakeReleaseSender{},
			unsubscribeBase:   "https://example.com/unsubscribe",
			wantSentCount:     0,
			wantMarkedCount:   0,
			wantNoSendOnError: true,
		},
		{
			name: "invalid unsubscribe base triggers url build failures",
			repo: &fakeNotifierRepository{
				pending: pending,
			},
			sender:            &fakeReleaseSender{},
			unsubscribeBase:   "://bad-url",
			wantSentCount:     0,
			wantMarkedCount:   0,
			wantNoSendOnError: true,
		},
		{
			name: "send failure does not mark notified and continues",
			repo: &fakeNotifierRepository{
				pending: pending,
			},
			sender: &fakeReleaseSender{
				sendErrByEmail: map[string]error{"b@example.com": errors.New("smtp error")},
			},
			unsubscribeBase:  "https://example.com/unsubscribe",
			wantSentCount:    3,
			wantMarkedCount:  2,
			wantMarkedSubIDs: []int64{1, 3},
			wantFirstURL:     "https://example.com/unsubscribe/token-a",
			wantFirstEmail:   "a@example.com",
		},
		{
			name: "mark failure continues with next subscription",
			repo: &fakeNotifierRepository{
				pending:        pending,
				markErrBySubID: map[int64]error{2: errors.New("update failed")},
			},
			sender:           &fakeReleaseSender{},
			unsubscribeBase:  "https://example.com/unsubscribe",
			wantSentCount:    3,
			wantMarkedCount:  3,
			wantMarkedSubIDs: []int64{1, 2, 3},
			wantFirstURL:     "https://example.com/unsubscribe/token-a",
			wantFirstEmail:   "a@example.com",
		},
		{
			name: "all success sends and marks all",
			repo: &fakeNotifierRepository{
				pending: pending,
			},
			sender:           &fakeReleaseSender{},
			unsubscribeBase:  "https://example.com/unsubscribe",
			wantSentCount:    3,
			wantMarkedCount:  3,
			wantMarkedSubIDs: []int64{1, 2, 3},
			wantFirstURL:     "https://example.com/unsubscribe/token-a",
			wantFirstEmail:   "a@example.com",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			runner := notifier.NewRunner(
				slog.New(slog.DiscardHandler),
				tc.repo,
				tc.sender,
				time.Second,
				tc.unsubscribeBase,
			)

			runner.RunOnce(t.Context())

			assert.Len(t, tc.sender.sent, tc.wantSentCount)
			assert.Len(t, tc.repo.marked, tc.wantMarkedCount)

			if tc.wantFirstURL != "" {
				require.NotEmpty(t, tc.sender.sent)
				assert.Equal(t, tc.wantFirstURL, tc.sender.sent[0].UnsubscribeURL)
				assert.Equal(t, tc.wantFirstEmail, tc.sender.sent[0].Email)
			}

			if tc.wantMarkedSubIDs != nil {
				got := make([]int64, 0, len(tc.repo.marked))
				for i := range tc.repo.marked {
					got = append(got, tc.repo.marked[i].subscriptionID)
				}
				assert.Equal(t, tc.wantMarkedSubIDs, got)
			}

			if tc.wantNoSendOnError {
				assert.Empty(t, tc.sender.sent)
			}
		})
	}
}

func TestRunner_String(t *testing.T) {
	t.Parallel()

	runner := notifier.NewRunner(
		slog.New(slog.DiscardHandler),
		&fakeNotifierRepository{},
		&fakeReleaseSender{},
		5*time.Second,
		"https://example.com/unsubscribe",
	)

	assert.Equal(t, "notifier(interval=5s)", runner.String())
}
