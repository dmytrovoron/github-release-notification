package repository

import (
	"time"

	app "github.com/dmytrovoron/github-release-notification/internal"
)

const (
	RepositoryTagInitialized RepositoryTagUpdateResult = "initialized"
	RepositoryTagChanged     RepositoryTagUpdateResult = "changed"
	RepositoryTagUnchanged   RepositoryTagUpdateResult = "unchanged"
)

type RepositoryTagUpdateResult string

type Subscription struct {
	ID               int64
	Email            string
	Repository       string
	LastSeenTag      string
	Status           app.SubscriptionStatus
	ConfirmToken     string
	UnsubscribeToken string
	CreatedAt        time.Time
	UpdatedAt        time.Time
}
