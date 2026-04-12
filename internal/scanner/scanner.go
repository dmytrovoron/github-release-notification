package scanner

import (
	"context"
	"fmt"
	"log/slog"
	"net/url"
	"strings"
	"time"

	"github.com/dmytrovoron/github-release-notification/internal/notifier"
	"github.com/dmytrovoron/github-release-notification/internal/repository"
)

type GitHubClient interface {
	LatestReleaseTag(ctx context.Context, owner, repo string) (string, error)
}

type ScannerRepository interface {
	ListActive(ctx context.Context) ([]repository.Subscription, error)
	AdvanceRepositoryTag(
		ctx context.Context,
		repositoryName string,
		tag string,
	) (repository.RepositoryTagUpdateResult, error)
}

type ReleaseSender interface {
	SendRelease(ctx context.Context, email notifier.ReleaseEmail) error
}

type Runner struct {
	log                *slog.Logger
	repo               ScannerRepository
	github             GitHubClient
	sender             ReleaseSender
	interval           time.Duration
	unsubscribeURLBase string
}

func NewRunner(
	log *slog.Logger,
	repo ScannerRepository,
	github GitHubClient,
	sender ReleaseSender,
	interval time.Duration,
	unsubscribeURLBase string,
) *Runner {
	if interval <= 0 {
		interval = time.Minute
	}

	return &Runner{
		log:                log,
		repo:               repo,
		github:             github,
		sender:             sender,
		interval:           interval,
		unsubscribeURLBase: unsubscribeURLBase,
	}
}

func (r *Runner) Start(ctx context.Context) {
	r.log.InfoContext(ctx, "starting release scanner", "interval", r.interval)
	r.runScan(ctx)

	ticker := time.NewTicker(r.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			r.log.InfoContext(ctx, "release scanner stopped")

			return
		case <-ticker.C:
			r.runScan(ctx)
		}
	}
}

func (r *Runner) String() string {
	return fmt.Sprintf("scanner(interval=%s)", r.interval)
}

func (r *Runner) RunOnce(ctx context.Context) {
	r.runScan(ctx)
}

func (r *Runner) runScan(ctx context.Context) {
	subscriptions, err := r.repo.ListActive(ctx)
	if err != nil {
		r.log.ErrorContext(ctx, "list active subscriptions for scanner", "error", err)

		return
	}

	if len(subscriptions) == 0 {
		return
	}

	byRepository := make(map[string][]repository.Subscription)
	for i := range subscriptions {
		sub := subscriptions[i]
		byRepository[sub.Repository] = append(byRepository[sub.Repository], sub)
	}

	for repositoryName, repoSubscribers := range byRepository {
		owner, repoName, ok := strings.Cut(repositoryName, "/")
		if !ok {
			r.log.WarnContext(ctx, "skip invalid repository name in active subscription", "repository", repositoryName)

			continue
		}

		tag, err := r.github.LatestReleaseTag(ctx, owner, repoName)
		if err != nil {
			r.log.ErrorContext(ctx, "fetch latest release tag", "repository", repositoryName, "error", err)

			continue
		}
		if strings.TrimSpace(tag) == "" {
			continue
		}

		updateResult, err := r.repo.AdvanceRepositoryTag(ctx, repositoryName, tag)
		if err != nil {
			r.log.ErrorContext(ctx, "advance repository tag", "repository", repositoryName, "tag", tag, "error", err)

			continue
		}
		if updateResult != repository.RepositoryTagChanged {
			continue
		}

		for i := range repoSubscribers {
			sub := repoSubscribers[i]
			unsubscribeURL, err := url.JoinPath(r.unsubscribeURLBase, sub.UnsubscribeToken)
			if err != nil {
				r.log.ErrorContext(
					ctx,
					"build unsubscribe url",
					"repository", repositoryName,
					"subscription_id", sub.ID,
					"error", err,
				)

				continue
			}

			err = r.sender.SendRelease(ctx, notifier.ReleaseEmail{
				Email:          sub.Email,
				Repository:     repositoryName,
				Tag:            tag,
				UnsubscribeURL: unsubscribeURL,
			})
			if err != nil {
				r.log.ErrorContext(
					ctx,
					"send release notification",
					"repository", repositoryName,
					"subscription_id", sub.ID,
					"email", sub.Email,
					"tag", tag,
					"error", err,
				)

				continue
			}

			r.log.InfoContext(ctx, "release notification sent", "repository", repositoryName, "email", sub.Email, "tag", tag)
		}
	}
}
