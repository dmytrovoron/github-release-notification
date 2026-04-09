// This file is safe to edit. Once it exists it will not be overwritten.

package restapi

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-openapi/errors"
	"github.com/go-openapi/runtime"
	"github.com/go-openapi/runtime/middleware"

	"github.com/dmytrovoron/github-release-notification/internal/http/restapi/operations"
	"github.com/dmytrovoron/github-release-notification/internal/http/restapi/operations/subscription"
)

//go:generate go tool -modfile=../../../tools/go.mod github.com/go-swagger/go-swagger/cmd/swagger generate server --spec ../../../api/swagger.yaml --target ../ --name GitHubReleaseNotification --exclude-main

func configureFlags(api *operations.GitHubReleaseNotificationAPI) {
	// api.CommandLineOptionsGroups = []swag.CommandLineOptionsGroup{ ... }
	_ = api
}

func configureAPI(api *operations.GitHubReleaseNotificationAPI) http.Handler {
	return configureAPIWithHealthChecker(api, nil)
}

// NewHandler configures API handlers and returns middleware-wrapped handler with an injected health checker.
func NewHandler(api *operations.GitHubReleaseNotificationAPI, healthChecker func(context.Context) error) http.Handler {
	return configureAPIWithHealthChecker(api, healthChecker)
}

func configureAPIWithHealthChecker(api *operations.GitHubReleaseNotificationAPI, healthChecker func(context.Context) error) http.Handler {
	// Configure the API here
	api.ServeError = errors.ServeError

	// Set your custom logger if needed. Default one is log.Printf
	// Expected interface func(string, ...any)
	//
	// Example:
	// api.Logger = log.Printf

	api.UseSwaggerUI()
	// To continue using redoc as your UI, uncomment the following line
	// api.UseRedoc()

	api.JSONConsumer = runtime.JSONConsumer()
	api.UrlformConsumer = runtime.DiscardConsumer

	api.JSONProducer = runtime.JSONProducer()

	// You may change here the memory limit for this multipart form parser. Below is the default (32 MB).
	// subscription.SubscribeMaxParseMemory = 32 << 20

	if api.SubscriptionConfirmSubscriptionHandler == nil {
		api.SubscriptionConfirmSubscriptionHandler = subscription.ConfirmSubscriptionHandlerFunc(
			func(params subscription.ConfirmSubscriptionParams) middleware.Responder {
				_ = params

				return middleware.NotImplemented("operation subscription.ConfirmSubscription has not yet been implemented")
			},
		)
	}
	if api.SubscriptionGetSubscriptionsHandler == nil {
		api.SubscriptionGetSubscriptionsHandler = subscription.GetSubscriptionsHandlerFunc(
			func(params subscription.GetSubscriptionsParams) middleware.Responder {
				_ = params

				return middleware.NotImplemented("operation subscription.GetSubscriptions has not yet been implemented")
			},
		)
	}
	if api.SubscriptionSubscribeHandler == nil {
		api.SubscriptionSubscribeHandler = subscription.SubscribeHandlerFunc(
			func(params subscription.SubscribeParams) middleware.Responder {
				_ = params

				return middleware.NotImplemented("operation subscription.Subscribe has not yet been implemented")
			},
		)
	}
	if api.SubscriptionUnsubscribeHandler == nil {
		api.SubscriptionUnsubscribeHandler = subscription.UnsubscribeHandlerFunc(
			func(params subscription.UnsubscribeParams) middleware.Responder {
				_ = params

				return middleware.NotImplemented("operation subscription.Unsubscribe has not yet been implemented")
			},
		)
	}

	api.PreServerShutdown = func() {}

	api.ServerShutdown = func() {}

	return setupGlobalMiddleware(api.Serve(setupMiddlewares), healthChecker)
}

// The TLS configuration before HTTPS server starts.
func configureTLS(tlsConfig *tls.Config) {
	// Make all necessary changes to the TLS configuration here.
	_ = tlsConfig
}

// As soon as server is initialized but not run yet, this function will be called.
// If you need to modify a config, store server instance to stop it individually later, this is the place.
// This function can be called multiple times, depending on the number of serving schemes.
// The scheme value will be set accordingly: "http", "https" or "unix".
func configureServer(server *http.Server, scheme, addr string) {
	_ = server
	_ = scheme
	_ = addr
}

// The middleware configuration is for the handler executors. These do not apply to the swagger.json document.
// The middleware executes after routing but before authentication, binding and validation.
func setupMiddlewares(handler http.Handler) http.Handler {
	return handler
}

// The middleware configuration happens before anything, this middleware also applies to serving the swagger.json document.
// So this is a good place to plug in a panic handling middleware, logging and metrics.
func setupGlobalMiddleware(handler http.Handler, healthChecker func(context.Context) error) http.Handler {
	checker := healthChecker
	if checker == nil {
		checker = func(context.Context) error { return nil }
	}

	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/healthz" && request.Method == http.MethodGet {
			ctx, cancel := context.WithTimeout(request.Context(), 2*time.Second)
			defer cancel()

			statusCode := http.StatusOK
			status := "ok"
			if err := checker(ctx); err != nil {
				statusCode = http.StatusServiceUnavailable
				status = "degraded"
			}

			writer.Header().Set("Content-Type", "application/json")
			writer.WriteHeader(statusCode)
			_ = json.NewEncoder(writer).Encode(map[string]string{"status": status})

			return
		}

		handler.ServeHTTP(writer, request)
	})
}
