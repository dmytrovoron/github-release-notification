package api_test

import (
	"context"

	app "github.com/dmytrovoron/github-release-notification/internal"
	"github.com/dmytrovoron/github-release-notification/internal/repository"
	"github.com/dmytrovoron/github-release-notification/internal/service/notifier"
)

type fakeSubscriptionRepository struct {
	existsResult        bool
	existsErr           error
	createResult        repository.Subscription
	createErr           error
	findByConfirmResult repository.Subscription
	findByConfirmErr    error
	findByUnsubResult   repository.Subscription
	findByUnsubErr      error
	listResult          []repository.Subscription
	listErr             error
	updateStatusErr     error
}

func (f *fakeSubscriptionRepository) ExistsActiveOrPending(_ context.Context, _, _ string) (bool, error) {
	return f.existsResult, f.existsErr
}

func (f *fakeSubscriptionRepository) Create(_ context.Context, _ *repository.Subscription) (repository.Subscription, error) {
	return f.createResult, f.createErr
}

func (f *fakeSubscriptionRepository) FindByConfirmToken(_ context.Context, _ string) (repository.Subscription, error) {
	return f.findByConfirmResult, f.findByConfirmErr
}

func (f *fakeSubscriptionRepository) FindByUnsubscribeToken(_ context.Context, _ string) (repository.Subscription, error) {
	return f.findByUnsubResult, f.findByUnsubErr
}

func (f *fakeSubscriptionRepository) ListActiveByEmail(_ context.Context, _ string) ([]repository.Subscription, error) {
	return f.listResult, f.listErr
}

func (f *fakeSubscriptionRepository) UpdateStatus(_ context.Context, _ int64, _ app.SubscriptionStatus) error {
	return f.updateStatusErr
}

type fakeGitHubChecker struct {
	exists bool
	err    error
}

func (f *fakeGitHubChecker) RepositoryExists(_ context.Context, _, _ string) (bool, error) {
	return f.exists, f.err
}

type fakeConfirmationSender struct {
	err error
}

func (f *fakeConfirmationSender) SendConfirmation(_ context.Context, _ notifier.ConfirmationEmail) error {
	return f.err
}
