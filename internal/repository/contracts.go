package repository

import (
	"context"
	"time"
)

const (
	SubscriptionStatusPending      SubscriptionStatus = "pending"
	SubscriptionStatusActive       SubscriptionStatus = "active"
	SubscriptionStatusUnsubscribed SubscriptionStatus = "unsubscribed"
)

type SubscriptionStatus string

type Subscription struct {
	ID               int64
	Email            string
	Repository       string
	Status           SubscriptionStatus
	ConfirmToken     string
	UnsubscribeToken string
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

type RepositoryState struct {
	Repository    string
	LastSeenTag   string
	LastCheckedAt time.Time
	UpdatedAt     time.Time
}

type SubscriptionRepository interface {
	Create(ctx context.Context, subscription Subscription) (Subscription, error)
	FindByConfirmToken(ctx context.Context, token string) (Subscription, error)
	FindByUnsubscribeToken(ctx context.Context, token string) (Subscription, error)
	ListActiveByEmail(ctx context.Context, email string) ([]Subscription, error)
	UpdateStatus(ctx context.Context, id int64, status SubscriptionStatus) error
}

type RepositoryStateRepository interface {
	GetByRepository(ctx context.Context, repository string) (RepositoryState, error)
	Upsert(ctx context.Context, state RepositoryState) error
}
