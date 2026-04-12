package postgres

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/dmytrovoron/github-release-notification/internal/repository"
)

type ScannerRepository struct {
	db *sql.DB
}

func NewScannerRepository(db *sql.DB) *ScannerRepository {
	return &ScannerRepository{db: db}
}

func (r *ScannerRepository) ListActive(ctx context.Context) ([]repository.Subscription, error) {
	const query = `
		SELECT id, email, repository, status, confirm_token, unsubscribe_token, created_at, updated_at
		FROM subscriptions
		WHERE status = 'active'
	`

	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("query active subscriptions: %w", err)
	}
	defer func() {
		_ = rows.Close()
	}()

	var result []repository.Subscription
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
			return nil, fmt.Errorf("scan active subscription row: %w", scanErr)
		}

		result = append(result, item)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate active subscription rows: %w", err)
	}

	return result, nil
}

func (r *ScannerRepository) AdvanceRepositoryTag(
	ctx context.Context,
	repositoryName string,
	tag string,
) (repository.RepositoryTagUpdateResult, error) {
	const insertQuery = `
		INSERT INTO repository_states (repository, last_seen_tag, last_checked_at, updated_at)
		VALUES ($1, $2, NOW(), NOW())
		ON CONFLICT (repository) DO NOTHING
	`

	insertRes, err := r.db.ExecContext(ctx, insertQuery, repositoryName, tag)
	if err != nil {
		return "", fmt.Errorf("initialize repository state: %w", err)
	}

	insertedRows, err := insertRes.RowsAffected()
	if err != nil {
		return "", fmt.Errorf("count initialized repository states: %w", err)
	}
	if insertedRows > 0 {
		return repository.RepositoryTagInitialized, nil
	}

	const updateQuery = `
		UPDATE repository_states
		SET last_seen_tag = $2,
			last_checked_at = NOW(),
			updated_at = NOW()
		WHERE repository = $1 AND last_seen_tag <> $2
	`

	updateRes, err := r.db.ExecContext(ctx, updateQuery, repositoryName, tag)
	if err != nil {
		return "", fmt.Errorf("update repository state tag: %w", err)
	}

	updatedRows, err := updateRes.RowsAffected()
	if err != nil {
		return "", fmt.Errorf("count updated repository states: %w", err)
	}
	if updatedRows > 0 {
		return repository.RepositoryTagChanged, nil
	}

	const touchQuery = `
		UPDATE repository_states
		SET last_checked_at = NOW()
		WHERE repository = $1
	`

	if _, err := r.db.ExecContext(ctx, touchQuery, repositoryName); err != nil {
		return "", fmt.Errorf("touch repository state check timestamp: %w", err)
	}

	return repository.RepositoryTagUnchanged, nil
}
