package restapi

import (
	"cmp"
	"fmt"
	"net/http"
	"runtime"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type requestMetricKey struct {
	method string
	path   string
	status int
}

type requestMetricValue struct {
	count       uint64
	durationSum float64
}

type metricsRegistry struct {
	now       func() time.Time
	startedAt time.Time

	inflight atomic.Int64

	mu       sync.Mutex
	requests map[requestMetricKey]requestMetricValue
}

func newMetricsRegistry() *metricsRegistry {
	return &metricsRegistry{
		now:       time.Now,
		startedAt: time.Now(),
		requests:  make(map[requestMetricKey]requestMetricValue),
	}
}

func (m *metricsRegistry) observe(method, path string, status int, duration time.Duration) {
	key := requestMetricKey{method: method, path: normalizePath(path), status: status}

	m.mu.Lock()
	value := m.requests[key]
	value.count++
	value.durationSum += duration.Seconds()
	m.requests[key] = value
	m.mu.Unlock()
}

func (m *metricsRegistry) renderPrometheus() string {
	uptimeSeconds := m.now().Sub(m.startedAt).Seconds()
	if uptimeSeconds < 0 {
		uptimeSeconds = 0
	}

	inflight := max(m.inflight.Load(), 0)

	m.mu.Lock()
	requestKeys := make([]requestMetricKey, 0, len(m.requests))
	for key := range m.requests {
		requestKeys = append(requestKeys, key)
	}

	slices.SortFunc(requestKeys, func(a, b requestMetricKey) int {
		return cmp.Or(cmp.Compare(a.path, b.path), cmp.Compare(a.method, b.method), cmp.Compare(a.status, b.status))
	})

	requestValues := make([]requestMetricValue, 0, len(requestKeys))
	for _, key := range requestKeys {
		requestValues = append(requestValues, m.requests[key])
	}
	m.mu.Unlock()

	var b strings.Builder
	b.WriteString("# HELP ghrn_service_uptime_seconds Service uptime in seconds.\n")
	b.WriteString("# TYPE ghrn_service_uptime_seconds gauge\n")
	fmt.Fprintf(&b, "ghrn_service_uptime_seconds %.6f\n", uptimeSeconds)

	b.WriteString("# HELP ghrn_go_goroutines Number of goroutines.\n")
	b.WriteString("# TYPE ghrn_go_goroutines gauge\n")
	fmt.Fprintf(&b, "ghrn_go_goroutines %d\n", runtime.NumGoroutine())

	b.WriteString("# HELP ghrn_http_inflight_requests Current number of in-flight HTTP requests.\n")
	b.WriteString("# TYPE ghrn_http_inflight_requests gauge\n")
	fmt.Fprintf(&b, "ghrn_http_inflight_requests %d\n", inflight)

	b.WriteString("# HELP ghrn_http_requests_total Total number of HTTP requests.\n")
	b.WriteString("# TYPE ghrn_http_requests_total counter\n")
	for idx, key := range requestKeys {
		value := requestValues[idx]
		labels := renderRequestLabels(key)
		fmt.Fprintf(&b, "ghrn_http_requests_total{%s} %d\n", labels, value.count)
	}

	b.WriteString("# HELP ghrn_http_request_duration_seconds_sum Total HTTP request duration in seconds.\n")
	b.WriteString("# TYPE ghrn_http_request_duration_seconds_sum counter\n")
	for idx, key := range requestKeys {
		value := requestValues[idx]
		labels := renderRequestLabels(key)
		fmt.Fprintf(&b, "ghrn_http_request_duration_seconds_sum{%s} %.6f\n", labels, value.durationSum)
	}

	b.WriteString("# HELP ghrn_http_request_duration_seconds_count Total number of HTTP requests tracked for duration.\n")
	b.WriteString("# TYPE ghrn_http_request_duration_seconds_count counter\n")
	for idx, key := range requestKeys {
		value := requestValues[idx]
		labels := renderRequestLabels(key)
		fmt.Fprintf(&b, "ghrn_http_request_duration_seconds_count{%s} %d\n", labels, value.count)
	}

	return b.String()
}

func metricsMiddleware(registry *metricsRegistry) func(http.Handler) http.Handler {
	if registry == nil {
		registry = newMetricsRegistry()
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			if request.Method == http.MethodGet && request.URL.Path == "/metrics" {
				writer.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
				_, _ = writer.Write([]byte(registry.renderPrometheus()))

				return
			}

			start := time.Now()
			registry.inflight.Add(1)
			defer registry.inflight.Add(-1)

			recorder := &statusRecorder{ResponseWriter: writer, status: http.StatusOK}
			next.ServeHTTP(recorder, request)

			registry.observe(request.Method, request.URL.Path, recorder.status, time.Since(start))
		})
	}
}

func renderRequestLabels(key requestMetricKey) string {
	return fmt.Sprintf("method=%q,path=%q,status=%d", escapeLabelValue(key.method), escapeLabelValue(key.path), key.status)
}

func escapeLabelValue(value string) string {
	value = strings.ReplaceAll(value, `\\`, `\\\\`)
	value = strings.ReplaceAll(value, "\n", `\\n`)

	return strings.ReplaceAll(value, `"`, `\\"`)
}

func normalizePath(path string) string {
	switch {
	case path == "":
		return "/"
	case strings.HasPrefix(path, "/api/confirm/"):
		return "/api/confirm/{token}"
	case strings.HasPrefix(path, "/api/unsubscribe/"):
		return "/api/unsubscribe/{token}"
	case strings.HasPrefix(path, "/confirm/"):
		return "/confirm/{token}"
	case strings.HasPrefix(path, "/unsubscribe/"):
		return "/unsubscribe/{token}"
	default:
		return path
	}
}
