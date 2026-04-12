//go:build e2e

package e2e

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	gh "github.com/google/go-github/v84/github"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/network"
	"github.com/testcontainers/testcontainers-go/wait"

	subclient "github.com/dmytrovoron/github-release-notification/tests/http/client"
)

// e is the package-level e2e fixture initialized once in TestMain.
//
//nolint:gochecknoglobals // intentional: single shared fixture set up in TestMain
var e e2e

func TestMain(m *testing.M) {
	os.Exit(run(m))
}

func run(m *testing.M) int {
	setupDockerEnv()

	githubAuthToken := strings.TrimSpace(os.Getenv("GITHUB_AUTH_TOKEN"))
	if githubAuthToken == "" {
		// https://github.com/settings/personal-access-tokens/new
		log.Println("skip: GITHUB_AUTH_TOKEN not set; provide a token with access to dmytrovoron/github-release-notification-e2e-test")

		return 0
	}

	if err := checkDockerProvider(); err != nil {
		log.Printf("skip: Docker provider not healthy: %v", err)

		return 0
	}

	ctx := context.Background()
	repoRoot := mustFindRepoRoot()

	nw, err := network.New(ctx, network.WithAttachable())
	if err != nil {
		log.Fatalf("create docker network: %v", err)
	}

	defer func() { _ = nw.Remove(ctx) }()

	dbC := mustStartPostgresContainer(ctx, nw)
	defer func() { _ = testcontainers.TerminateContainer(dbC) }()

	smtpC := mustStartSMTPContainer(ctx, nw)
	defer func() { _ = testcontainers.TerminateContainer(smtpC) }()

	appC := mustStartAppContainer(ctx, repoRoot, nw, githubAuthToken)
	defer func() { _ = appC.Terminate(ctx) }()

	dbHost, err := dbC.Host(ctx)
	if err != nil {
		log.Fatalf("resolve db host: %v", err)
	}

	dbPort, err := dbC.MappedPort(ctx, "5432/tcp")
	if err != nil {
		log.Fatalf("resolve db port: %v", err)
	}

	appHost, err := appC.Host(ctx)
	if err != nil {
		log.Fatalf("resolve app host: %v", err)
	}

	appPort, err := appC.MappedPort(ctx, "8080/tcp")
	if err != nil {
		log.Fatalf("resolve app port: %v", err)
	}

	smtpHost, err := smtpC.Host(ctx)
	if err != nil {
		log.Fatalf("resolve smtp host: %v", err)
	}

	smtpHTTPPort, err := smtpC.MappedPort(ctx, "8025/tcp")
	if err != nil {
		log.Fatalf("resolve smtp http port: %v", err)
	}

	databaseURL := fmt.Sprintf("postgres://app:app@%s/app?sslmode=disable", net.JoinHostPort(dbHost, dbPort.Port()))

	db, err := sql.Open("pgx", databaseURL)
	if err != nil {
		log.Fatalf("open db: %v", err)
	}

	defer func() { _ = db.Close() }()

	ghClient := gh.NewClient(&http.Client{Timeout: 20 * time.Second}).WithAuthToken(githubAuthToken)

	e = e2e{
		client:         mustNewAPIClient(fmt.Sprintf("http://%s/api", net.JoinHostPort(appHost, appPort.Port()))),
		db:             db,
		smtpAPIBaseURL: "http://" + net.JoinHostPort(smtpHost, smtpHTTPPort.Port()),
		gh:             ghClient,
	}

	if err := e.requireRepositoryAccess(ctx, scannerE2ERepository); err != nil {
		log.Fatalf("repository access check failed: %v", err)
	}

	code := m.Run()

	if code != 0 {
		dumpContainerLogs(ctx, appC, "app")
	}

	return code
}

func setupDockerEnv() {
	dockerHost := os.Getenv("DOCKER_HOST")
	if dockerHost == "" {
		homeDir, err := os.UserHomeDir()
		if err == nil {
			colimaSocket := filepath.Join(homeDir, ".colima", "default", "docker.sock")
			if _, statErr := os.Stat(colimaSocket); statErr == nil {
				dockerHost = "unix://" + colimaSocket
				_ = os.Setenv("DOCKER_HOST", dockerHost)
			}
		}
	}

	// On Colima, Ryuk must mount an in-VM socket path, not the host user path.
	if strings.Contains(dockerHost, "/.colima/") {
		_ = os.Setenv("TESTCONTAINERS_DOCKER_SOCKET_OVERRIDE", "/var/run/docker.sock")
	}
}

func checkDockerProvider() error {
	p, err := testcontainers.NewDockerProvider()
	if err != nil {
		return err
	}

	defer func() { _ = p.Close() }()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	return p.Health(ctx)
}

func mustFindRepoRoot() string {
	wd, err := os.Getwd()
	if err != nil {
		log.Fatalf("get working directory: %v", err)
	}

	current := wd
	for {
		goMod := filepath.Join(current, "go.mod")
		dockerfile := filepath.Join(current, "Dockerfile")

		if _, err := os.Stat(goMod); err == nil {
			if _, err := os.Stat(dockerfile); err == nil {
				return current
			}
		}

		parent := filepath.Dir(current)
		if parent == current {
			log.Fatalf("could not find directory containing both go.mod and Dockerfile from %q", wd)
		}

		current = parent
	}
}

func mustStartPostgresContainer(ctx context.Context, nw *testcontainers.DockerNetwork) testcontainers.Container {
	c, err := testcontainers.Run(
		ctx,
		"postgres:18-alpine",
		network.WithNetwork([]string{"db"}, nw),
		testcontainers.WithEnv(map[string]string{
			"POSTGRES_DB":       "app",
			"POSTGRES_USER":     "app",
			"POSTGRES_PASSWORD": "app",
		}),
		testcontainers.WithExposedPorts("5432/tcp"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").WithOccurrence(1).WithStartupTimeout(10*time.Second),
		),
	)
	if err != nil {
		log.Fatalf("start db container: %v", err)
	}

	return c
}

func mustStartSMTPContainer(ctx context.Context, nw *testcontainers.DockerNetwork) testcontainers.Container {
	c, err := testcontainers.Run(
		ctx,
		"axllent/mailpit:v1.27",
		network.WithNetwork([]string{"smtp"}, nw),
		testcontainers.WithExposedPorts("1025/tcp", "8025/tcp"),
		testcontainers.WithWaitStrategy(wait.ForHTTP("/api/v1/info").WithPort("8025/tcp").WithStartupTimeout(20*time.Second)),
	)
	if err != nil {
		log.Fatalf("start smtp container: %v", err)
	}

	return c
}

func mustStartAppContainer(
	ctx context.Context,
	repoRoot string,
	nw *testcontainers.DockerNetwork,
	githubAuthToken string,
) testcontainers.Container {
	appReq := testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			FromDockerfile: testcontainers.FromDockerfile{
				Context:    repoRoot,
				Dockerfile: "Dockerfile",
			},
			Env: map[string]string{
				"DATABASE_URL":          "postgres://app:app@db:5432/app?sslmode=disable",
				"MIGRATIONS_PATH":       "file:///app/migrations",
				"DATABASE_PING_TIMEOUT": "10s",
				"GITHUB_API_TIMEOUT":    "5s",
				"GITHUB_AUTH_TOKEN":     githubAuthToken,
				"SMTP_HOST":             "smtp",
				"SMTP_PORT":             "1025",
				"SCANNER_INTERVAL":      "2s",
				"NOTIFIER_INTERVAL":     "2s",
			},
			ExposedPorts: []string{"8080/tcp"},
			WaitingFor: wait.ForHTTP("/healthz").
				WithPort("8080/tcp").
				WithStatusCodeMatcher(func(code int) bool { return code == http.StatusOK }).
				WithStartupTimeout(120 * time.Second),
			Networks: []string{nw.Name},
			NetworkAliases: map[string][]string{
				nw.Name: {"app"},
			},
		},
		Started: true,
	}

	c, err := testcontainers.GenericContainer(ctx, appReq)
	if err != nil {
		log.Fatalf("start app container: %v", err)
	}

	return c
}

func mustNewAPIClient(baseURL string) *subclient.GitHubReleaseNotificationAPI {
	parsedURL, err := url.Parse(baseURL)
	if err != nil {
		log.Fatalf("parse api base url: %v", err)
	}

	if parsedURL.Host == "" {
		log.Fatalf("api base url host must not be empty")
	}

	basePath := parsedURL.Path
	if basePath == "" {
		basePath = "/"
	}

	cfg := subclient.DefaultTransportConfig().
		WithHost(parsedURL.Host).
		WithBasePath(basePath).
		WithSchemes([]string{parsedURL.Scheme})

	return subclient.NewHTTPClientWithConfig(nil, cfg)
}

func dumpContainerLogs(ctx context.Context, c testcontainers.Container, containerName string) {
	logCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	reader, err := c.Logs(logCtx)
	if err != nil {
		log.Printf("failed to read %s container logs: %v", containerName, err)

		return
	}

	defer func() { _ = reader.Close() }()

	logBytes, err := io.ReadAll(reader)
	if err != nil {
		log.Printf("failed to consume %s container logs: %v", containerName, err)

		return
	}

	const maxLogBytes = 200_000
	if len(logBytes) > maxLogBytes {
		log.Printf(
			"%s container logs (truncated, showing last %d of %d bytes):\n%s",
			containerName,
			maxLogBytes,
			len(logBytes),
			string(logBytes[len(logBytes)-maxLogBytes:]),
		)

		return
	}

	log.Printf("%s container logs:\n%s", containerName, string(logBytes))
}
