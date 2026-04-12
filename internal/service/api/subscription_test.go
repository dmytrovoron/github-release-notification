package api_test

import (
	"database/sql"
	"errors"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	app "github.com/dmytrovoron/github-release-notification/internal"
	"github.com/dmytrovoron/github-release-notification/internal/repository"
	"github.com/dmytrovoron/github-release-notification/internal/service/api"
)

func newService(repo *fakeSubscriptionRepository, gh *fakeGitHubChecker, sender *fakeConfirmationSender) *api.SubscriptionService {
	return api.NewSubscriptionService(
		repo,
		gh,
		sender,
		slog.New(slog.DiscardHandler),
		"https://example.com/confirm/",
	)
}

func TestSubscriptionService_Subscribe_EmailValidation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		email string
		valid bool
	}{
		{name: "simple", email: "user@example.com", valid: true},
		{name: "with plus tag", email: "user+tag@example.com", valid: true},
		{name: "subdomain", email: "user.name@sub.domain.com", valid: true},
		{name: "empty", email: "", valid: false},
		{name: "no domain", email: "notanemail", valid: false},
		{name: "no local part", email: "@nodomain.com", valid: false},
		{name: "missing domain", email: "missing@", valid: false},
		{name: "display name form", email: "John Doe <john@example.com>", valid: false},
		{name: "angle brackets only", email: "<john@example.com>", valid: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			svc := newService(&fakeSubscriptionRepository{}, &fakeGitHubChecker{}, &fakeConfirmationSender{})
			err := svc.Subscribe(t.Context(), tc.email, "owner/repo")
			if tc.valid {
				assert.NotErrorIs(t, err, api.ErrInvalidEmail)
			} else {
				assert.ErrorIs(t, err, api.ErrInvalidEmail)
			}
		})
	}
}

func TestSubscriptionService_Subscribe_RepositoryValidation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		repo  string
		valid bool
	}{
		{name: "simple", repo: "golang/go", valid: true},
		{name: "hyphens and dots", repo: "owner/repo-name.git", valid: true},
		{name: "underscores", repo: "my_org/my_repo", valid: true},
		{name: "digits", repo: "org123/repo456", valid: true},
		{name: "no slash", repo: "noslash", valid: false},
		{name: "empty", repo: "", valid: false},
		{name: "only slash", repo: "/", valid: false},
		{name: "missing owner", repo: "/repo", valid: false},
		{name: "missing repo", repo: "owner/", valid: false},
		{name: "special chars", repo: "owner!x/repo", valid: false},
		{name: "three parts", repo: "a/b/c", valid: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			svc := newService(&fakeSubscriptionRepository{}, &fakeGitHubChecker{}, &fakeConfirmationSender{})
			err := svc.Subscribe(t.Context(), "user@example.com", tc.repo)
			if tc.valid {
				assert.NotErrorIs(t, err, api.ErrInvalidRepository)
			} else {
				assert.ErrorIs(t, err, api.ErrInvalidRepository)
			}
		})
	}
}

func TestSubscriptionService_Subscribe_ExistsCheckError(t *testing.T) {
	t.Parallel()

	repoErr := errors.New("db error")
	repo := &fakeSubscriptionRepository{existsErr: repoErr}
	svc := newService(repo, &fakeGitHubChecker{}, &fakeConfirmationSender{})

	err := svc.Subscribe(t.Context(), "user@example.com", "owner/repo")
	require.ErrorContains(t, err, "check existing subscription")
	assert.ErrorIs(t, err, repoErr)
}

func TestSubscriptionService_Subscribe_AlreadySubscribed(t *testing.T) {
	t.Parallel()

	repo := &fakeSubscriptionRepository{existsResult: true}
	svc := newService(repo, &fakeGitHubChecker{}, &fakeConfirmationSender{})

	err := svc.Subscribe(t.Context(), "user@example.com", "owner/repo")
	assert.ErrorIs(t, err, api.ErrSubscriptionConflict)
}

func TestSubscriptionService_Subscribe_GitHubCheckError(t *testing.T) {
	t.Parallel()

	ghErr := errors.New("github unavailable")
	repo := &fakeSubscriptionRepository{existsResult: false}
	gh := &fakeGitHubChecker{err: ghErr}
	svc := newService(repo, gh, &fakeConfirmationSender{})

	err := svc.Subscribe(t.Context(), "user@example.com", "owner/repo")
	require.ErrorContains(t, err, "check repository in github")
	assert.ErrorIs(t, err, ghErr)
}

func TestSubscriptionService_Subscribe_RepositoryNotFound(t *testing.T) {
	t.Parallel()

	repo := &fakeSubscriptionRepository{existsResult: false}
	gh := &fakeGitHubChecker{exists: false}
	svc := newService(repo, gh, &fakeConfirmationSender{})

	err := svc.Subscribe(t.Context(), "user@example.com", "owner/repo")
	assert.ErrorIs(t, err, api.ErrRepositoryNotFound)
}

func TestSubscriptionService_Subscribe_CreateError(t *testing.T) {
	t.Parallel()

	createErr := errors.New("insert failed")
	repo := &fakeSubscriptionRepository{existsResult: false, createErr: createErr}
	gh := &fakeGitHubChecker{exists: true}
	svc := newService(repo, gh, &fakeConfirmationSender{})

	err := svc.Subscribe(t.Context(), "user@example.com", "owner/repo")
	require.ErrorContains(t, err, "create subscription")
	assert.ErrorIs(t, err, createErr)
}

func TestSubscriptionService_Subscribe_EmailSendFailureIsIgnored(t *testing.T) {
	t.Parallel()

	repo := &fakeSubscriptionRepository{existsResult: false}
	gh := &fakeGitHubChecker{exists: true}
	sender := &fakeConfirmationSender{err: errors.New("smtp down")}
	svc := newService(repo, gh, sender)

	// Email failure must not surface as an error.
	err := svc.Subscribe(t.Context(), "user@example.com", "owner/repo")
	assert.NoError(t, err)
}

func TestSubscriptionService_Subscribe_HappyPath(t *testing.T) {
	t.Parallel()

	repo := &fakeSubscriptionRepository{existsResult: false}
	gh := &fakeGitHubChecker{exists: true}
	svc := newService(repo, gh, &fakeConfirmationSender{})

	err := svc.Subscribe(t.Context(), "user@example.com", "owner/repo")
	assert.NoError(t, err)
}

func TestSubscriptionService_Confirm_TokenValidation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		token string
		valid bool
	}{
		{name: "valid 16-char hex", token: "0123456789abcdef", valid: true},
		{name: "valid uppercase hex", token: "0123456789ABCDEF", valid: true},
		{name: "too short", token: "0123456789abcde", valid: false},
		{name: "too long", token: "0123456789abcdef0", valid: false},
		{name: "empty", token: "", valid: false},
		{name: "non-hex chars", token: "0123456789abcdeg", valid: false},
		{name: "with spaces", token: "0123456789abcde ", valid: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			svc := newService(&fakeSubscriptionRepository{}, &fakeGitHubChecker{}, &fakeConfirmationSender{})
			err := svc.Confirm(t.Context(), tc.token)
			if tc.valid {
				assert.NotErrorIs(t, err, api.ErrInvalidToken)
			} else {
				assert.ErrorIs(t, err, api.ErrInvalidToken)
			}
		})
	}
}

func TestSubscriptionService_Confirm_TokenNotFound(t *testing.T) {
	t.Parallel()

	repo := &fakeSubscriptionRepository{findByConfirmErr: sql.ErrNoRows}
	svc := newService(repo, &fakeGitHubChecker{}, &fakeConfirmationSender{})

	err := svc.Confirm(t.Context(), "0123456789abcdef")
	assert.ErrorIs(t, err, api.ErrTokenNotFound)
}

func TestSubscriptionService_Confirm_FindError(t *testing.T) {
	t.Parallel()

	dbErr := errors.New("db down")
	repo := &fakeSubscriptionRepository{findByConfirmErr: dbErr}
	svc := newService(repo, &fakeGitHubChecker{}, &fakeConfirmationSender{})

	err := svc.Confirm(t.Context(), "0123456789abcdef")
	require.ErrorContains(t, err, "find subscription by confirm token")
	assert.ErrorIs(t, err, dbErr)
}

func TestSubscriptionService_Confirm_UpdateStatusError(t *testing.T) {
	t.Parallel()

	updateErr := errors.New("update failed")
	repo := &fakeSubscriptionRepository{
		findByConfirmResult: repository.Subscription{ID: 42},
		updateStatusErr:     updateErr,
	}
	svc := newService(repo, &fakeGitHubChecker{}, &fakeConfirmationSender{})

	err := svc.Confirm(t.Context(), "0123456789abcdef")
	require.Error(t, err)
	assert.ErrorIs(t, err, updateErr)
}

func TestSubscriptionService_Confirm_HappyPath(t *testing.T) {
	t.Parallel()

	repo := &fakeSubscriptionRepository{
		findByConfirmResult: repository.Subscription{ID: 1},
	}
	svc := newService(repo, &fakeGitHubChecker{}, &fakeConfirmationSender{})

	err := svc.Confirm(t.Context(), "0123456789abcdef")
	assert.NoError(t, err)
}

func TestSubscriptionService_Unsubscribe_TokenValidation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		token string
		valid bool
	}{
		{name: "valid 16-char hex", token: "0123456789abcdef", valid: true},
		{name: "valid uppercase hex", token: "0123456789ABCDEF", valid: true},
		{name: "too short", token: "0123456789abcde", valid: false},
		{name: "too long", token: "0123456789abcdef0", valid: false},
		{name: "empty", token: "", valid: false},
		{name: "non-hex chars", token: "0123456789abcdeg", valid: false},
		{name: "with spaces", token: "0123456789abcde ", valid: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			svc := newService(&fakeSubscriptionRepository{}, &fakeGitHubChecker{}, &fakeConfirmationSender{})
			err := svc.Unsubscribe(t.Context(), tc.token)
			if tc.valid {
				assert.NotErrorIs(t, err, api.ErrInvalidToken)
			} else {
				assert.ErrorIs(t, err, api.ErrInvalidToken)
			}
		})
	}
}

func TestSubscriptionService_Unsubscribe_TokenNotFound(t *testing.T) {
	t.Parallel()

	repo := &fakeSubscriptionRepository{findByUnsubErr: sql.ErrNoRows}
	svc := newService(repo, &fakeGitHubChecker{}, &fakeConfirmationSender{})

	err := svc.Unsubscribe(t.Context(), "0123456789abcdef")
	assert.ErrorIs(t, err, api.ErrTokenNotFound)
}

func TestSubscriptionService_Unsubscribe_FindError(t *testing.T) {
	t.Parallel()

	dbErr := errors.New("db down")
	repo := &fakeSubscriptionRepository{findByUnsubErr: dbErr}
	svc := newService(repo, &fakeGitHubChecker{}, &fakeConfirmationSender{})

	err := svc.Unsubscribe(t.Context(), "0123456789abcdef")
	require.ErrorContains(t, err, "find subscription by unsubscribe token")
	assert.ErrorIs(t, err, dbErr)
}

func TestSubscriptionService_Unsubscribe_UpdateStatusError(t *testing.T) {
	t.Parallel()

	updateErr := errors.New("update failed")
	repo := &fakeSubscriptionRepository{
		findByUnsubResult: repository.Subscription{ID: 7},
		updateStatusErr:   updateErr,
	}
	svc := newService(repo, &fakeGitHubChecker{}, &fakeConfirmationSender{})

	err := svc.Unsubscribe(t.Context(), "0123456789abcdef")
	require.Error(t, err)
	assert.ErrorIs(t, err, updateErr)
}

func TestSubscriptionService_Unsubscribe_HappyPath(t *testing.T) {
	t.Parallel()

	repo := &fakeSubscriptionRepository{
		findByUnsubResult: repository.Subscription{ID: 3},
	}
	svc := newService(repo, &fakeGitHubChecker{}, &fakeConfirmationSender{})

	err := svc.Unsubscribe(t.Context(), "0123456789abcdef")
	assert.NoError(t, err)
}

func TestSubscriptionService_ListByEmail_EmailValidation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		email string
		valid bool
	}{
		{name: "simple", email: "user@example.com", valid: true},
		{name: "with plus tag", email: "user+tag@example.com", valid: true},
		{name: "subdomain", email: "user.name@sub.domain.com", valid: true},
		{name: "trimmed whitespace", email: "  user@example.com  ", valid: true},
		{name: "empty", email: "", valid: false},
		{name: "no domain", email: "notanemail", valid: false},
		{name: "no local part", email: "@nodomain.com", valid: false},
		{name: "missing domain", email: "missing@", valid: false},
		{name: "display name form", email: "John Doe <john@example.com>", valid: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			svc := newService(&fakeSubscriptionRepository{}, &fakeGitHubChecker{}, &fakeConfirmationSender{})
			_, err := svc.ListByEmail(t.Context(), tc.email)
			if tc.valid {
				assert.NotErrorIs(t, err, api.ErrInvalidEmail)
			} else {
				assert.ErrorIs(t, err, api.ErrInvalidEmail)
			}
		})
	}
}

func TestSubscriptionService_ListByEmail_RepositoryError(t *testing.T) {
	t.Parallel()

	dbErr := errors.New("db down")
	repo := &fakeSubscriptionRepository{listErr: dbErr}
	svc := newService(repo, &fakeGitHubChecker{}, &fakeConfirmationSender{})

	_, err := svc.ListByEmail(t.Context(), "user@example.com")
	require.ErrorContains(t, err, "list active subscriptions by email")
	assert.ErrorIs(t, err, dbErr)
}

func TestSubscriptionService_ListByEmail_MapsStatusToConfirmed(t *testing.T) {
	t.Parallel()

	repo := &fakeSubscriptionRepository{
		listResult: []repository.Subscription{
			{Email: "user@example.com", Repository: "owner/a", Status: app.SubscriptionStatusActive, LastSeenTag: "v1.0"},
			{Email: "user@example.com", Repository: "owner/b", Status: app.SubscriptionStatusPending, LastSeenTag: ""},
			{Email: "user@example.com", Repository: "owner/c", Status: app.SubscriptionStatusUnsubscribed, LastSeenTag: "v2.0"},
		},
	}
	svc := newService(repo, &fakeGitHubChecker{}, &fakeConfirmationSender{})

	result, err := svc.ListByEmail(t.Context(), "user@example.com")
	require.NoError(t, err)

	expected := []app.Subscription{
		{Email: "user@example.com", Repository: "owner/a", Confirmed: true, LastSeenTag: "v1.0"},
		{Email: "user@example.com", Repository: "owner/b", Confirmed: false, LastSeenTag: ""},
		{Email: "user@example.com", Repository: "owner/c", Confirmed: false, LastSeenTag: "v2.0"},
	}
	assert.Equal(t, expected, result)
}

func TestSubscriptionService_ListByEmail_EmptyResultSet(t *testing.T) {
	t.Parallel()

	repo := &fakeSubscriptionRepository{listResult: []repository.Subscription{}}
	svc := newService(repo, &fakeGitHubChecker{}, &fakeConfirmationSender{})

	result, err := svc.ListByEmail(t.Context(), "user@example.com")
	require.NoError(t, err)
	assert.Empty(t, result)
}
