package postgres

import (
	"context"
	"database/sql"
	"fmt"

	app "github.com/dmytrovoron/github-release-notification/internal"
	"github.com/dmytrovoron/github-release-notification/internal/repository"
)

type SubscriptionRepository struct {
	db *sql.DB
}

func NewSubscriptionRepository(db *sql.DB) *SubscriptionRepository {
	return &SubscriptionRepository{db: db}
}

func (r *SubscriptionRepository) Create(
	ctx context.Context,
	subscriptionEntity *repository.Subscription,
) (repository.Subscription, error) {
	const query = `
		INSERT INTO subscriptions (email, repository, status, confirm_token, unsubscribe_token)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, email, repository, status, confirm_token, unsubscribe_token, created_at, updated_at
	`

	var created repository.Subscription
	err := r.db.QueryRowContext(
		ctx,
		query,
		subscriptionEntity.Email,
		subscriptionEntity.Repository,
		subscriptionEntity.Status,
		subscriptionEntity.ConfirmToken,
		subscriptionEntity.UnsubscribeToken,
	).Scan(
		&created.ID,
		&created.Email,
		&created.Repository,
		&created.Status,
		&created.ConfirmToken,
		&created.UnsubscribeToken,
		&created.CreatedAt,
		&created.UpdatedAt,
	)
	if err != nil {
		return repository.Subscription{}, fmt.Errorf("insert subscription: %w", err)
	}

	return created, nil
}

func (r *SubscriptionRepository) ExistsActiveOrPending(ctx context.Context, email, repositoryName string) (bool, error) {
	const query = `
		SELECT EXISTS (
			SELECT 1
			FROM subscriptions
			WHERE email = $1 AND repository = $2 AND status IN ('pending', 'active')
		)
	`

	var exists bool
	if err := r.db.QueryRowContext(ctx, query, email, repositoryName).Scan(&exists); err != nil {
		return false, fmt.Errorf("check existing subscription: %w", err)
	}

	return exists, nil
}

func (r *SubscriptionRepository) FindByConfirmToken(ctx context.Context, token string) (repository.Subscription, error) {
	const query = `
		SELECT id, email, repository, status, confirm_token, unsubscribe_token, created_at, updated_at
		FROM subscriptions
		WHERE confirm_token = $1
	`

	var item repository.Subscription
	err := r.db.QueryRowContext(ctx, query, token).Scan(
		&item.ID,
		&item.Email,
		&item.Repository,
		&item.Status,
		&item.ConfirmToken,
		&item.UnsubscribeToken,
		&item.CreatedAt,
		&item.UpdatedAt,
	)
	if err != nil {
		return repository.Subscription{}, fmt.Errorf("find by confirm token: %w", err)
	}

	return item, nil
}

func (r *SubscriptionRepository) FindByUnsubscribeToken(ctx context.Context, token string) (repository.Subscription, error) {
	const query = `
		SELECT id, email, repository, status, confirm_token, unsubscribe_token, created_at, updated_at
		FROM subscriptions
		WHERE unsubscribe_token = $1
	`

	var item repository.Subscription
	err := r.db.QueryRowContext(ctx, query, token).Scan(
		&item.ID,
		&item.Email,
		&item.Repository,
		&item.Status,
		&item.ConfirmToken,
		&item.UnsubscribeToken,
		&item.CreatedAt,
		&item.UpdatedAt,
	)
	if err != nil {
		return repository.Subscription{}, fmt.Errorf("find by unsubscribe token: %w", err)
	}

	return item, nil
}

func (r *SubscriptionRepository) ListActiveByEmail(ctx context.Context, email string) ([]repository.Subscription, error) {
	const query = `
		SELECT id, email, repository, status, confirm_token, unsubscribe_token, created_at, updated_at
		FROM subscriptions
		WHERE email = $1 AND status = 'active'
		ORDER BY created_at DESC
	`

	rows, err := r.db.QueryContext(ctx, query, email)
	if err != nil {
		return nil, fmt.Errorf("query active subscriptions by email: %w", err)
	}
	defer func() {
		_ = rows.Close()
	}()

	result := make([]repository.Subscription, 0)
	for rows.Next() {
		var item repository.Subscription
		if scanErr := rows.Scan(
			&item.ID,
			&item.Email,
			&item.Repository,
			&item.Status,
			&item.ConfirmToken,
			&item.UnsubscribeToken,
			&item.CreatedAt,
			&item.UpdatedAt,
		); scanErr != nil {
			return nil, fmt.Errorf("scan subscription row: %w", scanErr)
		}

		result = append(result, item)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate subscription rows: %w", err)
	}

	return result, nil
}

func (r *SubscriptionRepository) UpdateStatus(
	ctx context.Context,
	id int64,
	status app.SubscriptionStatus,
) error {
	const query = `
		UPDATE subscriptions
		SET status = $2, updated_at = NOW()
		WHERE id = $1
	`

	_, err := r.db.ExecContext(ctx, query, id, status)
	if err != nil {
		return fmt.Errorf("update subscription status: %w", err)
	}

	return nil
}
