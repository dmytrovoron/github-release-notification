package scanner_test

import (
	"context"

	"github.com/dmytrovoron/github-release-notification/internal/repository"
)

type fakeGitHubClient struct {
	tags            map[string]string
	errByRepository map[string]error
	called          []string
}

func (f *fakeGitHubClient) LatestReleaseTag(_ context.Context, owner, repo string) (string, error) {
	repositoryName := owner + "/" + repo
	f.called = append(f.called, repositoryName)
	if err, ok := f.errByRepository[repositoryName]; ok {
		return "", err
	}

	return f.tags[repositoryName], nil
}

type fakeScannerRepository struct {
	subscriptions          []repository.Subscription
	listErr                error
	results                map[string]repository.RepositoryTagUpdateResult
	advanceErrByRepository map[string]error
	advanced               []string
}

func (f *fakeScannerRepository) ListActive(_ context.Context) ([]repository.Subscription, error) {
	if f.listErr != nil {
		return nil, f.listErr
	}

	return f.subscriptions, nil
}

func (f *fakeScannerRepository) AdvanceRepositoryTag(
	_ context.Context,
	repositoryName string,
	_ string,
) (repository.RepositoryTagUpdateResult, error) {
	f.advanced = append(f.advanced, repositoryName)
	if err, ok := f.advanceErrByRepository[repositoryName]; ok {
		return "", err
	}

	if result, ok := f.results[repositoryName]; ok {
		return result, nil
	}

	return repository.RepositoryTagUnchanged, nil
}
