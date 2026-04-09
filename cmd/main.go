package main

import (
	"errors"
	"log"
	"os"

	"github.com/go-openapi/loads"
	"github.com/jessevdk/go-flags"

	"github.com/dmytrovoron/github-release-notification/internal/http/restapi"
	"github.com/dmytrovoron/github-release-notification/internal/http/restapi/operations"
)

func main() {
	server()
}

func server() {
	swaggerSpec, err := loads.Embedded(restapi.SwaggerJSON, restapi.FlatSwaggerJSON)
	if err != nil {
		log.Fatalln(err)
	}

	api := operations.NewGitHubReleaseNotificationAPI(swaggerSpec)
	server := restapi.NewServer(api)
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

	server.ConfigureAPI() // configure handlers, routes and middleware

	if err := server.Serve(); err != nil {
		_ = server.Shutdown()

		log.Fatalln(err)
	}
}
