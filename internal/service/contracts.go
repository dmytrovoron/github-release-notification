package service

import "context"

type SubscriptionDTO struct {
	Email       string
	Repository  string
	Confirmed   bool
	LastSeenTag string
}

type SubscriptionService interface {
	Subscribe(ctx context.Context, email, repository string) error
	Confirm(ctx context.Context, token string) error
	Unsubscribe(ctx context.Context, token string) error
	ListByEmail(ctx context.Context, email string) ([]SubscriptionDTO, error)
}

type ReleaseScanner interface {
	Run(ctx context.Context) error
}
