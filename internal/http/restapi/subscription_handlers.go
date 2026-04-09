package restapi

import (
	"context"
	"errors"
	"net/http"

	"github.com/go-openapi/runtime/middleware"

	app "github.com/dmytrovoron/github-release-notification/internal"
	"github.com/dmytrovoron/github-release-notification/internal/http/models"
	"github.com/dmytrovoron/github-release-notification/internal/http/restapi/operations"
	"github.com/dmytrovoron/github-release-notification/internal/http/restapi/operations/subscription"
	"github.com/dmytrovoron/github-release-notification/internal/service"
)

type SubscriptionService interface {
	Subscribe(ctx context.Context, email, repository string) error
	Confirm(ctx context.Context, token string) error
	Unsubscribe(ctx context.Context, token string) error
	ListByEmail(ctx context.Context, email string) ([]app.Subscription, error)
}

func RegisterSubscriptionHandlers(api *operations.GitHubReleaseNotificationAPI, svc SubscriptionService) {
	api.SubscriptionSubscribeHandler = subscription.SubscribeHandlerFunc(
		func(params subscription.SubscribeParams) middleware.Responder {
			err := svc.Subscribe(params.HTTPRequest.Context(), params.Email, params.Repo)
			if err == nil {
				return subscription.NewSubscribeOK()
			}

			switch {
			case errors.Is(err, service.ErrInvalidEmail), errors.Is(err, service.ErrInvalidRepository):
				return subscription.NewSubscribeBadRequest()
			case errors.Is(err, service.ErrRepositoryNotFound):
				return subscription.NewSubscribeNotFound()
			case errors.Is(err, service.ErrSubscriptionConflict):
				return subscription.NewSubscribeConflict()
			default:
				return middleware.Error(http.StatusInternalServerError, err.Error())
			}
		},
	)

	api.SubscriptionConfirmSubscriptionHandler = subscription.ConfirmSubscriptionHandlerFunc(
		func(params subscription.ConfirmSubscriptionParams) middleware.Responder {
			err := svc.Confirm(params.HTTPRequest.Context(), params.Token)
			if err == nil {
				return subscription.NewConfirmSubscriptionOK()
			}

			switch {
			case errors.Is(err, service.ErrInvalidToken):
				return subscription.NewConfirmSubscriptionBadRequest()
			case errors.Is(err, service.ErrTokenNotFound):
				return subscription.NewConfirmSubscriptionNotFound()
			default:
				return middleware.Error(http.StatusInternalServerError, err.Error())
			}
		},
	)

	api.SubscriptionUnsubscribeHandler = subscription.UnsubscribeHandlerFunc(
		func(params subscription.UnsubscribeParams) middleware.Responder {
			err := svc.Unsubscribe(params.HTTPRequest.Context(), params.Token)
			if err == nil {
				return subscription.NewUnsubscribeOK()
			}

			switch {
			case errors.Is(err, service.ErrInvalidToken):
				return subscription.NewUnsubscribeBadRequest()
			case errors.Is(err, service.ErrTokenNotFound):
				return subscription.NewUnsubscribeNotFound()
			default:
				return middleware.Error(http.StatusInternalServerError, err.Error())
			}
		},
	)

	api.SubscriptionGetSubscriptionsHandler = subscription.GetSubscriptionsHandlerFunc(
		func(params subscription.GetSubscriptionsParams) middleware.Responder {
			items, err := svc.ListByEmail(params.HTTPRequest.Context(), params.Email)
			if err != nil {
				if errors.Is(err, service.ErrInvalidEmail) {
					return subscription.NewGetSubscriptionsBadRequest()
				}

				return middleware.Error(http.StatusInternalServerError, err.Error())
			}

			payload := make([]*models.Subscription, 0, len(items))
			for _, item := range items {
				email := item.Email
				repo := item.Repository
				payload = append(payload, &models.Subscription{
					Email:       &email,
					Repo:        &repo,
					Confirmed:   item.Confirmed,
					LastSeenTag: item.LastSeenTag,
				})
			}

			return subscription.NewGetSubscriptionsOK().WithPayload(payload)
		},
	)
}
