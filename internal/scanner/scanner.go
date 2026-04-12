package scanner

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

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

type Runner struct {
	log      *slog.Logger
	repo     ScannerRepository
	github   GitHubClient
	interval time.Duration
}

type scanStats struct {
	activeSubscriptions     int
	repositories            int
	changedRepositories     int
	initializedRepositories int
	unchangedRepositories   int
	invalidRepositories     int
	githubFailures          int
	advanceFailures         int
}

func NewRunner(
	log *slog.Logger,
	repo ScannerRepository,
	github GitHubClient,
	interval time.Duration,
) *Runner {
	return &Runner{
		log:      log,
		repo:     repo,
		github:   github,
		interval: interval,
	}
}

func (r *Runner) Start(ctx context.Context) {
	r.log.InfoContext(ctx, "starting release scanner", "interval", r.interval.String())
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
	startedAt := time.Now()
	stats := scanStats{}

	r.log.InfoContext(ctx, "scanner run started", "interval", r.interval.String())

	subscriptions, err := r.repo.ListActive(ctx)
	if err != nil {
		r.log.ErrorContext(ctx, "list active subscriptions for scanner", "duration", time.Since(startedAt).String(), "error", err)

		return
	}
	stats.activeSubscriptions = len(subscriptions)

	if len(subscriptions) == 0 {
		r.log.InfoContext(ctx, "scanner run completed", "duration", time.Since(startedAt), "active_subscriptions", 0, "repositories", 0)

		return
	}

	byRepository := make(map[string][]repository.Subscription)
	for i := range subscriptions {
		sub := subscriptions[i]
		byRepository[sub.Repository] = append(byRepository[sub.Repository], sub)
	}
	stats.repositories = len(byRepository)

	r.log.InfoContext(
		ctx,
		"scanner grouped active subscriptions",
		"active_subscriptions", stats.activeSubscriptions,
		"repositories", stats.repositories,
	)

	for repositoryName, repoSubscribers := range byRepository {
		r.log.InfoContext(
			ctx,
			"scanner checking repository",
			"repository", repositoryName,
			"subscriber_count", len(repoSubscribers),
		)

		owner, repoName, ok := strings.Cut(repositoryName, "/")
		if !ok {
			stats.invalidRepositories++
			r.log.WarnContext(ctx, "skip invalid repository name in active subscription", "repository", repositoryName)

			continue
		}

		tag, err := r.github.LatestReleaseTag(ctx, owner, repoName)
		if err != nil {
			stats.githubFailures++
			r.log.ErrorContext(ctx, "fetch latest release tag", "repository", repositoryName, "error", err)

			continue
		}

		r.log.InfoContext(ctx, "scanner fetched latest release tag", "repository", repositoryName, "tag", tag)

		updateResult, err := r.repo.AdvanceRepositoryTag(ctx, repositoryName, tag)
		if err != nil {
			stats.advanceFailures++
			r.log.ErrorContext(ctx, "advance repository tag", "repository", repositoryName, "tag", tag, "error", err)

			continue
		}

		switch updateResult {
		case repository.RepositoryTagChanged:
			stats.changedRepositories++
			r.log.InfoContext(
				ctx,
				"scanner detected new release",
				"repository", repositoryName,
				"tag", tag,
				"subscriber_count", len(repoSubscribers),
			)
		case repository.RepositoryTagInitialized:
			stats.initializedRepositories++
			r.log.InfoContext(ctx, "scanner initialized repository state", "repository", repositoryName, "tag", tag)
		case repository.RepositoryTagUnchanged:
			stats.unchangedRepositories++
			r.log.InfoContext(ctx, "scanner repository tag unchanged", "repository", repositoryName, "tag", tag)
		default:
			r.log.InfoContext(ctx, "scanner repository tag result", "repository", repositoryName, "tag", tag, "result", updateResult)
		}

		if updateResult != repository.RepositoryTagChanged {
			continue
		}
	}

	r.log.InfoContext(
		ctx,
		"scanner run completed",
		"duration", time.Since(startedAt),
		"active_subscriptions", stats.activeSubscriptions,
		"repositories", stats.repositories,
		"changed_repositories", stats.changedRepositories,
		"initialized_repositories", stats.initializedRepositories,
		"unchanged_repositories", stats.unchangedRepositories,
		"invalid_repositories", stats.invalidRepositories,
		"github_failures", stats.githubFailures,
		"advance_failures", stats.advanceFailures,
	)
}
