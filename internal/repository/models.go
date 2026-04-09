package repository

import (
	"time"

	app "github.com/dmytrovoron/github-release-notification/internal"
)

type Subscription struct {
	ID               int64
	Email            string
	Repository       string
	Status           app.SubscriptionStatus
	ConfirmToken     string
	UnsubscribeToken string
	CreatedAt        time.Time
	UpdatedAt        time.Time
}
