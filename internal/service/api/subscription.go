package api

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"net/mail"
	"net/url"
	"regexp"
	"strings"

	app "github.com/dmytrovoron/github-release-notification/internal"
	"github.com/dmytrovoron/github-release-notification/internal/repository"
	"github.com/dmytrovoron/github-release-notification/internal/service/notifier"
)

const maxTokenLength = 8

type GitHubRepositoryChecker interface {
	RepositoryExists(ctx context.Context, owner, repo string) (bool, error)
}

type ConfirmationSender interface {
	SendConfirmation(ctx context.Context, email notifier.ConfirmationEmail) error
}

type SubscriptionRepository interface {
	Create(ctx context.Context, subscription *repository.Subscription) (repository.Subscription, error)
	ExistsActiveOrPending(ctx context.Context, email, repository string) (bool, error)
	FindByConfirmToken(ctx context.Context, token string) (repository.Subscription, error)
	FindByUnsubscribeToken(ctx context.Context, token string) (repository.Subscription, error)
	ListActiveByEmail(ctx context.Context, email string) ([]repository.Subscription, error)
	UpdateStatus(ctx context.Context, id int64, status app.SubscriptionStatus) error
}

type SubscriptionService struct {
	subscriptions      SubscriptionRepository
	githubChecker      GitHubRepositoryChecker
	confirmationSender ConfirmationSender
	log                *slog.Logger
	confirmURLBase     string
}

var (
	ErrInvalidEmail         = errors.New("invalid email")
	ErrInvalidRepository    = errors.New("invalid repository")
	ErrInvalidToken         = errors.New("invalid token")
	ErrRepositoryNotFound   = errors.New("repository not found")
	ErrSubscriptionConflict = errors.New("subscription conflict")
	ErrTokenNotFound        = errors.New("token not found")
)

var repositoryPattern = regexp.MustCompile(`^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$`)

func NewSubscriptionService(
	subscriptions SubscriptionRepository,
	githubChecker GitHubRepositoryChecker,
	confirmationSender ConfirmationSender,
	log *slog.Logger,
	confirmURLBase string,
) *SubscriptionService {
	return &SubscriptionService{
		subscriptions:      subscriptions,
		githubChecker:      githubChecker,
		confirmationSender: confirmationSender,
		log:                log,
		confirmURLBase:     confirmURLBase,
	}
}

func (s *SubscriptionService) Subscribe(ctx context.Context, email, ownerRepo string) error {
	if !isValidEmail(email) {
		return ErrInvalidEmail
	}
	if !isValidRepository(ownerRepo) {
		return ErrInvalidRepository
	}

	alreadySubscribed, err := s.subscriptions.ExistsActiveOrPending(ctx, email, ownerRepo)
	if err != nil {
		return fmt.Errorf("check existing subscription: %w", err)
	}
	if alreadySubscribed {
		return ErrSubscriptionConflict
	}

	owner, repo, _ := strings.Cut(ownerRepo, "/")
	exists, err := s.githubChecker.RepositoryExists(ctx, owner, repo)
	if err != nil {
		return fmt.Errorf("check repository in github: %w", err)
	}
	if !exists {
		return ErrRepositoryNotFound
	}

	confirmToken, err := generateToken()
	if err != nil {
		return err
	}
	unsubscribeToken, err := generateToken()
	if err != nil {
		return err
	}

	confirmURL, err := url.JoinPath(s.confirmURLBase, confirmToken)
	if err != nil {
		return fmt.Errorf("build confirm url: %w", err)
	}

	_, err = s.subscriptions.Create(ctx, &repository.Subscription{
		Email:            email,
		Repository:       ownerRepo,
		Status:           app.SubscriptionStatusPending,
		ConfirmToken:     confirmToken,
		UnsubscribeToken: unsubscribeToken,
	})
	if err != nil {
		return fmt.Errorf("create subscription: %w", err)
	}

	err = s.confirmationSender.SendConfirmation(ctx, notifier.ConfirmationEmail{
		Email:        email,
		Repository:   ownerRepo,
		ConfirmToken: confirmToken,
		ConfirmURL:   confirmURL,
	})
	if err != nil {
		// Log email failure but don't fail the subscription creation
		s.log.ErrorContext(ctx, "Failed to send confirmation email", "email", email, "repo", ownerRepo, "error", err)
	}

	return nil
}

func (s *SubscriptionService) Confirm(ctx context.Context, token string) error {
	if !isValidToken(token) {
		return ErrInvalidToken
	}

	subscriptionEntity, err := s.subscriptions.FindByConfirmToken(ctx, token)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrTokenNotFound
		}

		return fmt.Errorf("find subscription by confirm token: %w", err)
	}

	if err := s.updateStatus(ctx, subscriptionEntity.ID, app.SubscriptionStatusActive); err != nil {
		return err
	}

	s.log.InfoContext(ctx, "Subscription confirmed", "subscriptionID", subscriptionEntity.ID)

	return nil
}

func (s *SubscriptionService) Unsubscribe(ctx context.Context, token string) error {
	if !isValidToken(token) {
		return ErrInvalidToken
	}

	subscriptionEntity, err := s.subscriptions.FindByUnsubscribeToken(ctx, token)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrTokenNotFound
		}

		return fmt.Errorf("find subscription by unsubscribe token: %w", err)
	}

	if err := s.updateStatus(ctx, subscriptionEntity.ID, app.SubscriptionStatusUnsubscribed); err != nil {
		return err
	}

	s.log.InfoContext(ctx, "Subscription unsubscribed", "subscriptionID", subscriptionEntity.ID)

	return nil
}

func (s *SubscriptionService) ListByEmail(ctx context.Context, email string) ([]app.Subscription, error) {
	email = strings.TrimSpace(email)
	if !isValidEmail(email) {
		return nil, ErrInvalidEmail
	}

	subscriptions, err := s.subscriptions.ListActiveByEmail(ctx, email)
	if err != nil {
		return nil, fmt.Errorf("list active subscriptions by email: %w", err)
	}

	result := make([]app.Subscription, 0, len(subscriptions))
	for i := range subscriptions {
		item := subscriptions[i]
		result = append(result, app.Subscription{
			Email:       item.Email,
			Repository:  item.Repository,
			Confirmed:   item.Status == app.SubscriptionStatusActive,
			LastSeenTag: item.LastSeenTag,
		})
	}

	return result, nil
}

func isValidEmail(email string) bool {
	parsed, err := mail.ParseAddress(email)
	if err != nil {
		return false
	}

	return parsed.Address == email
}

func isValidRepository(repositoryName string) bool {
	return repositoryPattern.MatchString(repositoryName)
}

func generateToken() (string, error) {
	buf := make([]byte, maxTokenLength)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate secure token bytes: %w", err)
	}

	return hex.EncodeToString(buf), nil
}

func isValidToken(token string) bool {
	if len(token) != maxTokenLength*2 {
		return false
	}

	_, err := hex.DecodeString(token)

	return err == nil
}

func (s *SubscriptionService) updateStatus(
	ctx context.Context,
	subscriptionID int64,
	status app.SubscriptionStatus,
) error {
	if err := s.subscriptions.UpdateStatus(ctx, subscriptionID, status); err != nil {
		return fmt.Errorf("update subscription status: %w", err)
	}

	return nil
}
