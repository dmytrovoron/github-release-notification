package restapi_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-openapi/loads"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dmytrovoron/github-release-notification/internal/service/api/restapi"
	"github.com/dmytrovoron/github-release-notification/internal/service/api/restapi/operations"
)

func TestDocsEndpointServesSwaggerUI(t *testing.T) {
	spec, err := loads.Embedded(restapi.SwaggerJSON, restapi.FlatSwaggerJSON)
	require.NoError(t, err, "load embedded spec")

	api := operations.NewGitHubReleaseNotificationAPI(spec)
	handler := restapi.NewHandler(api, nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/docs", http.NoBody)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code, "unexpected status code")

	contentType := rec.Header().Get("Content-Type")
	assert.Contains(t, contentType, "text/html", "expected HTML content type")

	body := strings.ToLower(rec.Body.String())
	assert.Contains(t, body, "<html", "expected HTML response body")
	assert.Contains(t, body, "swagger", "expected swagger docs page content")
}

func TestMetricsEndpoint_ExposesPrometheusIndicators(t *testing.T) {
	t.Parallel()

	spec, err := loads.Embedded(restapi.SwaggerJSON, restapi.FlatSwaggerJSON)
	require.NoError(t, err, "load embedded spec")

	api := operations.NewGitHubReleaseNotificationAPI(spec)
	handler := restapi.NewHandler(api, nil, nil)

	rootReq := httptest.NewRequest(http.MethodGet, "/", http.NoBody)
	rootRec := httptest.NewRecorder()
	handler.ServeHTTP(rootRec, rootReq)
	require.Equal(t, http.StatusOK, rootRec.Code)

	healthReq := httptest.NewRequest(http.MethodGet, "/healthz", http.NoBody)
	healthRec := httptest.NewRecorder()
	handler.ServeHTTP(healthRec, healthReq)
	require.Equal(t, http.StatusOK, healthRec.Code)

	metricsReq := httptest.NewRequest(http.MethodGet, "/metrics", http.NoBody)
	metricsRec := httptest.NewRecorder()
	handler.ServeHTTP(metricsRec, metricsReq)

	require.Equal(t, http.StatusOK, metricsRec.Code)
	assert.Equal(t, "text/plain; version=0.0.4; charset=utf-8", metricsRec.Header().Get("Content-Type"))

	body := metricsRec.Body.String()
	assert.Contains(t, body, "# HELP ghrn_service_uptime_seconds")
	assert.Contains(t, body, "# TYPE ghrn_service_uptime_seconds gauge")
	assert.Contains(t, body, "ghrn_go_goroutines")
	assert.Contains(t, body, "ghrn_http_inflight_requests 0")
	assert.Contains(t, body, `ghrn_http_requests_total{method="GET",path="/",status="200"} 1`)
	assert.Contains(t, body, `ghrn_http_requests_total{method="GET",path="/healthz",status="200"} 1`)
	assert.NotContains(t, body, `path="/metrics"`)

	assert.Regexp(t, `ghrn_http_request_duration_seconds_sum\{method="GET",path="/",status="200"\} [0-9]+\.[0-9]{6}`, body)
	assert.Regexp(t, `ghrn_http_request_duration_seconds_count\{method="GET",path="/healthz",status="200"\} 1`, body)
}
