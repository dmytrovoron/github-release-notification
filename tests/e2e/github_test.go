//go:build e2e

package e2e

import (
	"net"
	"net/http"
	"os"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"
)

func startGitHubStub(t *testing.T) string {
	t.Helper()

	mux := http.NewServeMux()
	mux.HandleFunc("/repos/owner/repo", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})

	srv := &http.Server{Handler: mux}
	var lcfg net.ListenConfig
	ln, err := lcfg.Listen(t.Context(), "tcp", "0.0.0.0:0")
	require.NoError(t, err, "listen github stub")

	go func() {
		_ = srv.Serve(ln)
	}()

	t.Cleanup(func() {
		_ = srv.Shutdown(t.Context())
	})

	addr, ok := ln.Addr().(*net.TCPAddr)
	require.True(t, ok, "resolve github stub port")
	host := os.Getenv("E2E_GITHUB_STUB_HOST")
	if host == "" {
		host = "host.docker.internal"
	}

	return "http://" + net.JoinHostPort(host, strconv.Itoa(addr.Port))
}
