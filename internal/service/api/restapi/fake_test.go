package restapi_test

import (
	"context"

	app "github.com/dmytrovoron/github-release-notification/internal"
)

type fakeSubscriptionService struct {
	subscribeErr   error
	confirmErr     error
	unsubscribeErr error
	listErr        error
	listItems      []app.Subscription

	subscribeEmail string
	subscribeRepo  string
	confirmToken   string
	unsubToken     string
	listEmail      string
}

func (f *fakeSubscriptionService) Subscribe(_ context.Context, email, ownerRepo string) error {
	f.subscribeEmail = email
	f.subscribeRepo = ownerRepo

	return f.subscribeErr
}

func (f *fakeSubscriptionService) Confirm(_ context.Context, token string) error {
	f.confirmToken = token

	return f.confirmErr
}

func (f *fakeSubscriptionService) Unsubscribe(_ context.Context, token string) error {
	f.unsubToken = token

	return f.unsubscribeErr
}

func (f *fakeSubscriptionService) ListByEmail(_ context.Context, email string) ([]app.Subscription, error) {
	f.listEmail = email
	if f.listErr != nil {
		return nil, f.listErr
	}

	return f.listItems, nil
}
