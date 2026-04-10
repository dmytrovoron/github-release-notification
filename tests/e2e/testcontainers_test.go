//go:build e2e

package e2e

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/network"
	"github.com/testcontainers/testcontainers-go/wait"
)

func setupE2EEnv(t *testing.T) e2eEnv {
	t.Helper()

	if testing.Short() {
		t.Skip("skipping e2e test in short mode")
	}
	configureTestcontainersDockerEnv(t)
	testcontainers.SkipIfProviderIsNotHealthy(t)

	ctx := t.Context()
	repoRoot := findRepoRoot(t)
	githubBaseURL := startGitHubStub(t)

	nw, err := network.New(ctx, network.WithAttachable())
	require.NoError(t, err, "create docker network")
	t.Cleanup(func() {
		_ = nw.Remove(ctx)
	})

	dbC, err := startPostgresContainer(ctx, nw)
	require.NoError(t, err, "start db container")
	t.Cleanup(func() {
		_ = testcontainers.TerminateContainer(dbC)
	})

	dbHost, err := dbC.Host(ctx)
	require.NoError(t, err, "resolve db host")
	dbPort, err := dbC.MappedPort(ctx, "5432/tcp")
	require.NoError(t, err, "resolve db port")

	appC, err := startAppContainer(ctx, repoRoot, nw, githubBaseURL)
	require.NoError(t, err, "start app container")
	t.Cleanup(func() {
		_ = appC.Terminate(ctx)
	})

	appHost, err := appC.Host(ctx)
	require.NoError(t, err, "resolve app host")
	appPort, err := appC.MappedPort(ctx, "8080/tcp")
	require.NoError(t, err, "resolve app port")

	return e2eEnv{
		client:             &http.Client{Timeout: 10 * time.Second},
		baseURL:            fmt.Sprintf("http://%s/api", net.JoinHostPort(appHost, appPort.Port())),
		databaseURLForTest: fmt.Sprintf("postgres://app:app@%s/app?sslmode=disable", net.JoinHostPort(dbHost, dbPort.Port())),
	}
}

func configureTestcontainersDockerEnv(t *testing.T) {
	t.Helper()

	dockerHost := os.Getenv("DOCKER_HOST")
	if dockerHost == "" {
		homeDir, err := os.UserHomeDir()
		if err == nil {
			colimaSocket := filepath.Join(homeDir, ".colima", "default", "docker.sock")
			if _, statErr := os.Stat(colimaSocket); statErr == nil {
				dockerHost = "unix://" + colimaSocket
				t.Setenv("DOCKER_HOST", dockerHost)
			}
		}
	}

	// On Colima, Ryuk must mount an in-VM socket path, not the host user path.
	if strings.Contains(dockerHost, "/.colima/") {
		t.Setenv("TESTCONTAINERS_DOCKER_SOCKET_OVERRIDE", "/var/run/docker.sock")
	}
}

func findRepoRoot(t *testing.T) string {
	t.Helper()

	wd, err := os.Getwd()
	require.NoError(t, err, "get working directory")

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
			require.Failf(t, "resolve repo root", "could not find directory containing both go.mod and Dockerfile from %q", wd)
		}
		current = parent
	}
}

func startPostgresContainer(ctx context.Context, nw *testcontainers.DockerNetwork) (testcontainers.Container, error) {
	return testcontainers.Run(
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
}

func startAppContainer(
	ctx context.Context,
	repoRoot string,
	nw *testcontainers.DockerNetwork,
	githubBaseURL string,
) (testcontainers.Container, error) {
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
				"GITHUB_API_BASE_URL":   githubBaseURL,
				"GITHUB_API_TIMEOUT":    "5s",
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

	return testcontainers.GenericContainer(ctx, appReq)
}
