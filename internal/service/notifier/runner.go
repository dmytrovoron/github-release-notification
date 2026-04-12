package notifier

import (
	"context"
	"fmt"
	"log/slog"
	"net/url"
	"time"

	"github.com/dmytrovoron/github-release-notification/internal/repository"
)

// NotifierRepository provides access to subscriptions that need release emails.
type NotifierRepository interface {
	ListPendingNotifications(ctx context.Context) ([]repository.PendingNotification, error)
	MarkNotified(ctx context.Context, subscriptionID int64, tag string) error
}

// ReleaseSender is the minimal capability Runner needs from a notifier.
type ReleaseSender interface {
	SendRelease(ctx context.Context, email ReleaseEmail) error
}

type notifyStats struct {
	pending          int
	sent             int
	sendFailures     int
	markFailures     int
	urlBuildFailures int
}

// Runner periodically queries for pending notifications and sends release emails.
type Runner struct {
	log                *slog.Logger
	repo               NotifierRepository
	notifier           ReleaseSender
	interval           time.Duration
	unsubscribeURLBase string
}

func NewRunner(
	log *slog.Logger,
	repo NotifierRepository,
	notifier ReleaseSender,
	interval time.Duration,
	unsubscribeURLBase string,
) *Runner {
	return &Runner{
		log:                log,
		repo:               repo,
		notifier:           notifier,
		interval:           interval,
		unsubscribeURLBase: unsubscribeURLBase,
	}
}

func (r *Runner) Start(ctx context.Context) {
	r.log.InfoContext(ctx, "Starting release notifier", "interval", r.interval.String())
	r.runNotify(ctx)

	ticker := time.NewTicker(r.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			r.log.InfoContext(ctx, "Release notifier stopped")

			return
		case <-ticker.C:
			r.runNotify(ctx)
		}
	}
}

func (r *Runner) String() string {
	return fmt.Sprintf("notifier(interval=%s)", r.interval)
}

func (r *Runner) RunOnce(ctx context.Context) {
	r.runNotify(ctx)
}

func (r *Runner) runNotify(ctx context.Context) {
	startedAt := time.Now()
	stats := notifyStats{}

	r.log.InfoContext(ctx, "Notifier run started")

	pending, err := r.repo.ListPendingNotifications(ctx)
	if err != nil {
		r.log.ErrorContext(ctx, "List pending notifications", "duration", time.Since(startedAt).String(), "error", err)

		return
	}
	stats.pending = len(pending)

	for i := range pending {
		n := pending[i]

		unsubscribeURL, err := url.JoinPath(r.unsubscribeURLBase, n.UnsubscribeToken)
		if err != nil {
			stats.urlBuildFailures++
			r.log.ErrorContext(
				ctx,
				"Build unsubscribe url",
				"subscription_id", n.SubscriptionID,
				"repository", n.Repository,
				"error", err,
			)

			continue
		}

		if err := r.notifier.SendRelease(ctx, ReleaseEmail{
			Email:          n.Email,
			Repository:     n.Repository,
			Tag:            n.CurrentTag,
			UnsubscribeURL: unsubscribeURL,
		}); err != nil {
			stats.sendFailures++
			r.log.ErrorContext(
				ctx,
				"Send release notification",
				"subscription_id", n.SubscriptionID,
				"repository", n.Repository,
				"email", n.Email,
				"tag", n.CurrentTag,
				"error", err,
			)

			continue
		}

		if err := r.repo.MarkNotified(ctx, n.SubscriptionID, n.CurrentTag); err != nil {
			stats.markFailures++
			r.log.ErrorContext(
				ctx,
				"Mark subscription notified",
				"subscription_id", n.SubscriptionID,
				"tag", n.CurrentTag,
				"error", err,
			)

			continue
		}

		stats.sent++
		r.log.InfoContext(
			ctx,
			"Release notification sent",
			"subscription_id", n.SubscriptionID,
			"repository", n.Repository,
			"email", n.Email,
			"tag", n.CurrentTag,
		)
	}

	r.log.InfoContext(
		ctx,
		"Notifier run completed",
		"duration", time.Since(startedAt),
		"pending", stats.pending,
		"sent", stats.sent,
		"send_failures", stats.sendFailures,
		"mark_failures", stats.markFailures,
		"url_build_failures", stats.urlBuildFailures,
	)
}
