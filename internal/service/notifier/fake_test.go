package notifier_test

import (
	"context"

	"github.com/dmytrovoron/github-release-notification/internal/repository"
	"github.com/dmytrovoron/github-release-notification/internal/service/notifier"
)

type fakeNotifierRepository struct {
	pending        []repository.PendingNotification
	listErr        error
	markErrBySubID map[int64]error
	marked         []markCall
}

type markCall struct {
	subscriptionID int64
	tag            string
}

func (f *fakeNotifierRepository) ListPendingNotifications(_ context.Context) ([]repository.PendingNotification, error) {
	if f.listErr != nil {
		return nil, f.listErr
	}

	return f.pending, nil
}

func (f *fakeNotifierRepository) MarkNotified(_ context.Context, subscriptionID int64, tag string) error {
	f.marked = append(f.marked, markCall{subscriptionID: subscriptionID, tag: tag})
	if err, ok := f.markErrBySubID[subscriptionID]; ok {
		return err
	}

	return nil
}

type fakeReleaseSender struct {
	sendErrByEmail map[string]error
	sent           []notifier.ReleaseEmail
}

func (f *fakeReleaseSender) SendRelease(_ context.Context, email notifier.ReleaseEmail) error {
	f.sent = append(f.sent, email)
	if err, ok := f.sendErrByEmail[email.Email]; ok {
		return err
	}

	return nil
}
