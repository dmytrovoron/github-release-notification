package main

import (
	"context"
	"database/sql"
	"errors"
	"log"
	"os"
	"time"

	"github.com/go-openapi/loads"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/jessevdk/go-flags"

	"github.com/dmytrovoron/github-release-notification/internal/config"
	"github.com/dmytrovoron/github-release-notification/internal/http/restapi"
	"github.com/dmytrovoron/github-release-notification/internal/http/restapi/operations"
	"github.com/dmytrovoron/github-release-notification/internal/integration/github"
	"github.com/dmytrovoron/github-release-notification/internal/migrations"
	"github.com/dmytrovoron/github-release-notification/internal/repository/postgres"
	"github.com/dmytrovoron/github-release-notification/internal/service"
)

func main() {
	server()
}

func server() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalln(err)
	}

	db, err := sql.Open("pgx", cfg.DatabaseURL)
	if err != nil {
		log.Fatalln(err)
	}
	defer func() {
		if closeErr := db.Close(); closeErr != nil {
			log.Printf("close db connection: %v", closeErr)
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), cfg.DatabasePingTimeout)
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		log.Fatalln(err)
	}

	if err := migrations.Run(cfg.DatabaseURL, cfg.MigrationsPath); err != nil {
		log.Fatalln(err)
	}

	swaggerSpec, err := loads.Embedded(restapi.SwaggerJSON, restapi.FlatSwaggerJSON)
	if err != nil {
		log.Fatalln(err)
	}

	api := operations.NewGitHubReleaseNotificationAPI(swaggerSpec)
	githubClient, err := github.NewClient(cfg.GitHubAPIBaseURL, cfg.GitHubAPITimeout)
	if err != nil {
		log.Fatalln(err)
	}

	subscriptionRepo := postgres.NewSubscriptionRepository(db)
	subscriptionService := service.NewSubscriptionService(subscriptionRepo, githubClient)
	restapi.RegisterSubscriptionHandlers(api, subscriptionService)

	server := restapi.NewServer(api)
	server.SetHandler(restapi.NewHandler(api, func(checkCtx context.Context) error {
		pingCtx, pingCancel := context.WithTimeout(checkCtx, 2*time.Second)
		defer pingCancel()

		return db.PingContext(pingCtx)
	}))
	server.ConfigureFlags() // inject API-specific custom flags. Must be called before args parsing

	parser := flags.NewParser(server, flags.Default)
	parser.ShortDescription = "GitHub Release Notification API"
	parser.LongDescription = "GitHub Release Notification API that allows users to subscribe to email notifications " +
		"about new releases of a chosen GitHub repository."

	for _, optsGroup := range api.CommandLineOptionsGroups {
		_, err := parser.AddGroup(optsGroup.ShortDescription, optsGroup.LongDescription, optsGroup.Options)
		if err != nil {
			log.Fatalln(err)
		}
	}

	if _, err := parser.Parse(); err != nil {
		code := 1
		fe := new(flags.Error)
		if errors.As(err, &fe) {
			if fe.Type == flags.ErrHelp {
				code = 0
			}
		}
		os.Exit(code)
	}

	if err := server.Serve(); err != nil {
		_ = server.Shutdown()

		log.Fatalln(err)
	}
}
