package postgres

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/dmytrovoron/github-release-notification/internal/repository"
)

type NotifierRepository struct {
	db *sql.DB
}

func NewNotifierRepository(db *sql.DB) *NotifierRepository {
	return &NotifierRepository{db: db}
}

// ListPendingNotifications returns active subscriptions whose last_notified_tag
// differs from the current last_seen_tag in repository_states.
func (r *NotifierRepository) ListPendingNotifications(ctx context.Context) ([]repository.PendingNotification, error) {
	const query = `
		SELECT s.id, s.email, s.repository, s.unsubscribe_token, rs.last_seen_tag
		FROM subscriptions s
		JOIN repository_states rs ON rs.repository = s.repository
		WHERE s.status = 'active'
		  AND rs.last_seen_tag <> ''
		  AND s.last_notified_tag <> rs.last_seen_tag
	`

	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("query pending notifications: %w", err)
	}
	defer func() {
		_ = rows.Close()
	}()

	result := make([]repository.PendingNotification, 0)
	for rows.Next() {
		var item repository.PendingNotification
		if scanErr := rows.Scan(
			&item.SubscriptionID,
			&item.Email,
			&item.Repository,
			&item.UnsubscribeToken,
			&item.CurrentTag,
		); scanErr != nil {
			return nil, fmt.Errorf("scan pending notification row: %w", scanErr)
		}

		result = append(result, item)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate pending notification rows: %w", err)
	}

	return result, nil
}

// MarkNotified updates last_notified_tag for a subscription after an email was sent.
func (r *NotifierRepository) MarkNotified(ctx context.Context, subscriptionID int64, tag string) error {
	const query = `
		UPDATE subscriptions
		SET last_notified_tag = $2, updated_at = NOW()
		WHERE id = $1
	`

	_, err := r.db.ExecContext(ctx, query, subscriptionID, tag)
	if err != nil {
		return fmt.Errorf("mark subscription notified: %w", err)
	}

	return nil
}
