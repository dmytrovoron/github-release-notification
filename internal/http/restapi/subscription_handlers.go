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

type SubscriptionHandler struct {
	svc SubscriptionService
}

func NewSubscriptionHandler(svc SubscriptionService) *SubscriptionHandler {
	return &SubscriptionHandler{svc: svc}
}

func (h *SubscriptionHandler) Register(api *operations.GitHubReleaseNotificationAPI) {
	api.SubscriptionSubscribeHandler = subscription.SubscribeHandlerFunc(h.subscribe)
	api.SubscriptionConfirmSubscriptionHandler = subscription.ConfirmSubscriptionHandlerFunc(h.confirmSubscription)
	api.SubscriptionUnsubscribeHandler = subscription.UnsubscribeHandlerFunc(h.unsubscribe)
	api.SubscriptionGetSubscriptionsHandler = subscription.GetSubscriptionsHandlerFunc(h.getSubscriptions)
}

func (h *SubscriptionHandler) subscribe(params subscription.SubscribeParams) middleware.Responder {
	err := h.svc.Subscribe(params.HTTPRequest.Context(), params.Email, params.Repo)

	switch {
	case err == nil:
		return subscription.NewSubscribeOK()
	case errors.Is(err, service.ErrInvalidEmail), errors.Is(err, service.ErrInvalidRepository):
		return subscription.NewSubscribeBadRequest()
	case errors.Is(err, service.ErrRepositoryNotFound):
		return subscription.NewSubscribeNotFound()
	case errors.Is(err, service.ErrSubscriptionConflict):
		return subscription.NewSubscribeConflict()
	default:
		return middleware.Error(http.StatusInternalServerError, err.Error())
	}
}

func (h *SubscriptionHandler) confirmSubscription(params subscription.ConfirmSubscriptionParams) middleware.Responder {
	err := h.svc.Confirm(params.HTTPRequest.Context(), params.Token)

	switch {
	case err == nil:
		return subscription.NewConfirmSubscriptionOK()
	case errors.Is(err, service.ErrInvalidToken):
		return subscription.NewConfirmSubscriptionBadRequest()
	case errors.Is(err, service.ErrTokenNotFound):
		return subscription.NewConfirmSubscriptionNotFound()
	default:
		return middleware.Error(http.StatusInternalServerError, err.Error())
	}
}

func (h *SubscriptionHandler) unsubscribe(params subscription.UnsubscribeParams) middleware.Responder {
	err := h.svc.Unsubscribe(params.HTTPRequest.Context(), params.Token)

	switch {
	case err == nil:
		return subscription.NewUnsubscribeOK()
	case errors.Is(err, service.ErrInvalidToken):
		return subscription.NewUnsubscribeBadRequest()
	case errors.Is(err, service.ErrTokenNotFound):
		return subscription.NewUnsubscribeNotFound()
	default:
		return middleware.Error(http.StatusInternalServerError, err.Error())
	}
}

func (h *SubscriptionHandler) getSubscriptions(params subscription.GetSubscriptionsParams) middleware.Responder {
	items, err := h.svc.ListByEmail(params.HTTPRequest.Context(), params.Email)
	switch {
	case err == nil:
	case errors.Is(err, service.ErrInvalidEmail):
		return subscription.NewGetSubscriptionsBadRequest()
	default:
		return middleware.Error(http.StatusInternalServerError, err.Error())
	}

	payload := make([]*models.Subscription, 0, len(items))
	for _, item := range items {
		payload = append(payload, &models.Subscription{
			Email:       &item.Email,
			Repo:        &item.Repository,
			Confirmed:   item.Confirmed,
			LastSeenTag: item.LastSeenTag,
		})
	}

	return subscription.NewGetSubscriptionsOK().WithPayload(payload)
}
