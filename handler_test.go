package status

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync/atomic"
	"testing"

	jobsApi "github.com/roadrunner-server/api-plugins/v6/jobs"
	apiStatus "github.com/roadrunner-server/api-plugins/v6/status"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// --- Mock implementations ---

type mockChecker struct {
	name string
	st   *apiStatus.Status
	err  error
}

func (m *mockChecker) Status() (*apiStatus.Status, error) { return m.st, m.err }
func (m *mockChecker) Name() string                       { return m.name }

type mockReadiness struct {
	name string
	st   *apiStatus.Status
	err  error
}

func (m *mockReadiness) Ready() (*apiStatus.Status, error) { return m.st, m.err }
func (m *mockReadiness) Name() string                      { return m.name }

type mockJobsChecker struct {
	states []*jobsApi.State
	err    error
}

func (m *mockJobsChecker) JobsState(_ context.Context) ([]*jobsApi.State, error) {
	return m.states, m.err
}
func (m *mockJobsChecker) Name() string { return "jobs" }

// --- Helpers ---

func newShutdownPtr(val bool) *atomic.Pointer[bool] {
	var p atomic.Pointer[bool]
	p.Store(&val)
	return &p
}

func parseReports(t *testing.T, body []byte) []*Report {
	t.Helper()
	var reports []*Report
	require.NoError(t, json.Unmarshal(body, &reports))
	return reports
}

func parseJobsReports(t *testing.T, body []byte) []*JobsReport {
	t.Helper()
	var reports []*JobsReport
	require.NoError(t, json.Unmarshal(body, &reports))
	return reports
}

// --- Health Handler Tests ---

func TestHealthHandler(t *testing.T) {
	log := zap.NewNop()

	// ---- All-plugins path (no query params) ----

	t.Run("Shutdown", func(t *testing.T) {
		h := NewHealthHandler(nil, newShutdownPtr(true), log, http.StatusServiceUnavailable)
		rec := httptest.NewRecorder()
		req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/health", nil)
		h.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)
		assert.Contains(t, rec.Body.String(), "service is shutting down")
	})

	t.Run("EmptyRegistry", func(t *testing.T) {
		h := NewHealthHandler(map[string]Checker{}, newShutdownPtr(false), log, http.StatusServiceUnavailable)
		rec := httptest.NewRecorder()
		req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/health", nil)
		h.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)
		reports := parseReports(t, rec.Body.Bytes())
		assert.Empty(t, reports)
	})

	t.Run("HealthyPlugin", func(t *testing.T) {
		registry := map[string]Checker{
			"http": &mockChecker{name: "http", st: &apiStatus.Status{Code: 200}},
		}
		h := NewHealthHandler(registry, newShutdownPtr(false), log, http.StatusServiceUnavailable)
		rec := httptest.NewRecorder()
		req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/health", nil)
		h.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)
		reports := parseReports(t, rec.Body.Bytes())
		require.Len(t, reports, 1)
		assert.Equal(t, "http", reports[0].PluginName)
		assert.Equal(t, 200, reports[0].StatusCode)
		assert.Empty(t, reports[0].ErrorMessage)
	})

	t.Run("CheckerError", func(t *testing.T) {
		registry := map[string]Checker{
			"http": &mockChecker{name: "http", err: errors.New("connection refused")},
		}
		h := NewHealthHandler(registry, newShutdownPtr(false), log, http.StatusServiceUnavailable)
		rec := httptest.NewRecorder()
		req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/health", nil)
		h.ServeHTTP(rec, req)

		// All-plugins path DOES call WriteHeader on error
		assert.Equal(t, http.StatusServiceUnavailable, rec.Code)
		reports := parseReports(t, rec.Body.Bytes())
		require.Len(t, reports, 1)
		assert.Equal(t, "connection refused", reports[0].ErrorMessage)
		assert.Equal(t, http.StatusServiceUnavailable, reports[0].StatusCode)
	})

	t.Run("NilStatus", func(t *testing.T) {
		registry := map[string]Checker{
			"http": &mockChecker{name: "http", st: nil, err: nil},
		}
		h := NewHealthHandler(registry, newShutdownPtr(false), log, http.StatusServiceUnavailable)
		rec := httptest.NewRecorder()
		req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/health", nil)
		h.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)
		reports := parseReports(t, rec.Body.Bytes())
		require.Len(t, reports, 1)
		assert.Contains(t, reports[0].ErrorMessage, "plugin is not available")
		assert.Equal(t, http.StatusServiceUnavailable, reports[0].StatusCode)
	})

	t.Run("NilPlugin", func(t *testing.T) {
		registry := map[string]Checker{
			"http": nil,
		}
		h := NewHealthHandler(registry, newShutdownPtr(false), log, http.StatusServiceUnavailable)
		rec := httptest.NewRecorder()
		req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/health", nil)
		h.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)
		reports := parseReports(t, rec.Body.Bytes())
		require.Len(t, reports, 1)
		assert.Equal(t, "plugin is nil or not initialized", reports[0].ErrorMessage)
		assert.Equal(t, http.StatusNotFound, reports[0].StatusCode)
	})

	t.Run("ServerError", func(t *testing.T) {
		registry := map[string]Checker{
			"http": &mockChecker{name: "http", st: &apiStatus.Status{Code: 500}},
		}
		h := NewHealthHandler(registry, newShutdownPtr(false), log, http.StatusServiceUnavailable)
		rec := httptest.NewRecorder()
		req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/health", nil)
		h.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusServiceUnavailable, rec.Code)
		reports := parseReports(t, rec.Body.Bytes())
		require.Len(t, reports, 1)
		assert.Equal(t, "internal server error, see logs", reports[0].ErrorMessage)
		assert.Equal(t, http.StatusServiceUnavailable, reports[0].StatusCode)
	})

	t.Run("UnexpectedCode", func(t *testing.T) {
		registry := map[string]Checker{
			"http": &mockChecker{name: "http", st: &apiStatus.Status{Code: 450}},
		}
		h := NewHealthHandler(registry, newShutdownPtr(false), log, http.StatusServiceUnavailable)
		rec := httptest.NewRecorder()
		req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/health", nil)
		h.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)
		reports := parseReports(t, rec.Body.Bytes())
		require.Len(t, reports, 1)
		assert.Equal(t, "unexpected status code", reports[0].ErrorMessage)
		assert.Equal(t, 450, reports[0].StatusCode)
	})

	// ---- Filtered path (with ?plugin= query) ----

	t.Run("Filtered_Exists", func(t *testing.T) {
		registry := map[string]Checker{
			"http": &mockChecker{name: "http", st: &apiStatus.Status{Code: 200}},
		}
		h := NewHealthHandler(registry, newShutdownPtr(false), log, http.StatusServiceUnavailable)
		rec := httptest.NewRecorder()
		req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/health?plugin=http", nil)
		h.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)
		reports := parseReports(t, rec.Body.Bytes())
		require.Len(t, reports, 1)
		assert.Equal(t, "http", reports[0].PluginName)
		assert.Equal(t, 200, reports[0].StatusCode)
	})

	t.Run("Filtered_NonExistent", func(t *testing.T) {
		registry := map[string]Checker{
			"http": &mockChecker{name: "http", st: &apiStatus.Status{Code: 200}},
		}
		h := NewHealthHandler(registry, newShutdownPtr(false), log, http.StatusServiceUnavailable)
		rec := httptest.NewRecorder()
		req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/health?plugin=nonexistent", nil)
		h.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)
		reports := parseReports(t, rec.Body.Bytes())
		assert.Empty(t, reports)
	})

	t.Run("Filtered_Error", func(t *testing.T) {
		registry := map[string]Checker{
			"http": &mockChecker{name: "http", err: errors.New("connection refused")},
		}
		h := NewHealthHandler(registry, newShutdownPtr(false), log, http.StatusServiceUnavailable)
		rec := httptest.NewRecorder()
		req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/health?plugin=http", nil)
		h.ServeHTTP(rec, req)

		// Filtered path does NOT call WriteHeader on error (differs from all-plugins path)
		assert.Equal(t, http.StatusOK, rec.Code)
		reports := parseReports(t, rec.Body.Bytes())
		require.Len(t, reports, 1)
		assert.Equal(t, "connection refused", reports[0].ErrorMessage)
		assert.Equal(t, http.StatusInternalServerError, reports[0].StatusCode)
	})

	t.Run("Filtered_NilStatus", func(t *testing.T) {
		registry := map[string]Checker{
			"http": &mockChecker{name: "http", st: nil, err: nil},
		}
		h := NewHealthHandler(registry, newShutdownPtr(false), log, http.StatusServiceUnavailable)
		rec := httptest.NewRecorder()
		req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/health?plugin=http", nil)
		h.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)
		reports := parseReports(t, rec.Body.Bytes())
		require.Len(t, reports, 1)
		assert.Equal(t, "plugin is not available", reports[0].ErrorMessage)
		assert.Equal(t, http.StatusServiceUnavailable, reports[0].StatusCode)
	})

	t.Run("Filtered_ServerError", func(t *testing.T) {
		registry := map[string]Checker{
			"http": &mockChecker{name: "http", st: &apiStatus.Status{Code: 500}},
		}
		h := NewHealthHandler(registry, newShutdownPtr(false), log, http.StatusServiceUnavailable)
		rec := httptest.NewRecorder()
		req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/health?plugin=http", nil)
		h.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusServiceUnavailable, rec.Code)
		reports := parseReports(t, rec.Body.Bytes())
		require.Len(t, reports, 1)
		assert.Equal(t, "internal server error, see logs", reports[0].ErrorMessage)
		assert.Equal(t, http.StatusServiceUnavailable, reports[0].StatusCode)
	})

	t.Run("Filtered_UnexpectedCode", func(t *testing.T) {
		registry := map[string]Checker{
			"http": &mockChecker{name: "http", st: &apiStatus.Status{Code: 450}},
		}
		h := NewHealthHandler(registry, newShutdownPtr(false), log, http.StatusServiceUnavailable)
		rec := httptest.NewRecorder()
		req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/health?plugin=http", nil)
		h.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)
		reports := parseReports(t, rec.Body.Bytes())
		require.Len(t, reports, 1)
		assert.Equal(t, "unexpected status code", reports[0].ErrorMessage)
		assert.Equal(t, 450, reports[0].StatusCode)
	})

	// ---- Custom unavailable status code ----

	t.Run("CustomUnavailableCode", func(t *testing.T) {
		registry := map[string]Checker{
			"http": &mockChecker{name: "http", err: errors.New("fail")},
		}
		h := NewHealthHandler(registry, newShutdownPtr(false), log, http.StatusInternalServerError)
		rec := httptest.NewRecorder()
		req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/health", nil)
		h.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusInternalServerError, rec.Code)
		reports := parseReports(t, rec.Body.Bytes())
		require.Len(t, reports, 1)
		assert.Equal(t, http.StatusInternalServerError, reports[0].StatusCode)
	})
}

// --- Ready Handler Tests ---

func TestReadyHandler(t *testing.T) {
	log := zap.NewNop()

	// ---- All-plugins path (no query params) ----

	t.Run("Shutdown", func(t *testing.T) {
		h := NewReadyHandler(nil, newShutdownPtr(true), log, http.StatusServiceUnavailable)
		rec := httptest.NewRecorder()
		req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/ready", nil)
		h.ServeHTTP(rec, req)

		// Ready shutdown returns 503 (unlike health which returns 200)
		assert.Equal(t, http.StatusServiceUnavailable, rec.Code)
		assert.Contains(t, rec.Body.String(), "service is shutting down")
	})

	t.Run("EmptyRegistry", func(t *testing.T) {
		h := NewReadyHandler(map[string]Readiness{}, newShutdownPtr(false), log, http.StatusServiceUnavailable)
		rec := httptest.NewRecorder()
		req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/ready", nil)
		h.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)
		reports := parseReports(t, rec.Body.Bytes())
		assert.Empty(t, reports)
	})

	t.Run("ReadyPlugin", func(t *testing.T) {
		registry := map[string]Readiness{
			"http": &mockReadiness{name: "http", st: &apiStatus.Status{Code: 200}},
		}
		h := NewReadyHandler(registry, newShutdownPtr(false), log, http.StatusServiceUnavailable)
		rec := httptest.NewRecorder()
		req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/ready", nil)
		h.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)
		reports := parseReports(t, rec.Body.Bytes())
		require.Len(t, reports, 1)
		assert.Equal(t, "http", reports[0].PluginName)
		assert.Equal(t, 200, reports[0].StatusCode)
		assert.Empty(t, reports[0].ErrorMessage)
	})

	t.Run("ReadinessError", func(t *testing.T) {
		registry := map[string]Readiness{
			"http": &mockReadiness{name: "http", err: errors.New("not ready")},
		}
		h := NewReadyHandler(registry, newShutdownPtr(false), log, http.StatusServiceUnavailable)
		rec := httptest.NewRecorder()
		req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/ready", nil)
		h.ServeHTTP(rec, req)

		// All-plugins path does NOT call WriteHeader on error (opposite of health)
		assert.Equal(t, http.StatusOK, rec.Code)
		reports := parseReports(t, rec.Body.Bytes())
		require.Len(t, reports, 1)
		assert.Equal(t, "not ready", reports[0].ErrorMessage)
		assert.Equal(t, http.StatusInternalServerError, reports[0].StatusCode)
	})

	t.Run("NilStatus", func(t *testing.T) {
		registry := map[string]Readiness{
			"http": &mockReadiness{name: "http", st: nil, err: nil},
		}
		h := NewReadyHandler(registry, newShutdownPtr(false), log, http.StatusServiceUnavailable)
		rec := httptest.NewRecorder()
		req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/ready", nil)
		h.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)
		reports := parseReports(t, rec.Body.Bytes())
		require.Len(t, reports, 1)
		assert.Equal(t, "plugin is not available", reports[0].ErrorMessage)
		assert.Equal(t, http.StatusServiceUnavailable, reports[0].StatusCode)
	})

	t.Run("NilPlugin", func(t *testing.T) {
		registry := map[string]Readiness{
			"http": nil,
		}
		h := NewReadyHandler(registry, newShutdownPtr(false), log, http.StatusServiceUnavailable)
		rec := httptest.NewRecorder()
		req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/ready", nil)
		h.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)
		reports := parseReports(t, rec.Body.Bytes())
		require.Len(t, reports, 1)
		assert.Equal(t, "plugin is nil or not initialized", reports[0].ErrorMessage)
		assert.Equal(t, http.StatusNotFound, reports[0].StatusCode)
	})

	t.Run("ServerError", func(t *testing.T) {
		registry := map[string]Readiness{
			"http": &mockReadiness{name: "http", st: &apiStatus.Status{Code: 500}},
		}
		h := NewReadyHandler(registry, newShutdownPtr(false), log, http.StatusServiceUnavailable)
		rec := httptest.NewRecorder()
		req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/ready", nil)
		h.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusServiceUnavailable, rec.Code)
		reports := parseReports(t, rec.Body.Bytes())
		require.Len(t, reports, 1)
		assert.Equal(t, "internal server error, see logs", reports[0].ErrorMessage)
		assert.Equal(t, http.StatusServiceUnavailable, reports[0].StatusCode)
	})

	t.Run("UnexpectedCode", func(t *testing.T) {
		registry := map[string]Readiness{
			"http": &mockReadiness{name: "http", st: &apiStatus.Status{Code: 450}},
		}
		h := NewReadyHandler(registry, newShutdownPtr(false), log, http.StatusServiceUnavailable)
		rec := httptest.NewRecorder()
		req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/ready", nil)
		h.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)
		reports := parseReports(t, rec.Body.Bytes())
		require.Len(t, reports, 1)
		assert.Equal(t, "unexpected status code", reports[0].ErrorMessage)
		assert.Equal(t, 450, reports[0].StatusCode)
	})

	// ---- Filtered path (with ?plugin= query) ----

	t.Run("Filtered_Exists", func(t *testing.T) {
		registry := map[string]Readiness{
			"http": &mockReadiness{name: "http", st: &apiStatus.Status{Code: 200}},
		}
		h := NewReadyHandler(registry, newShutdownPtr(false), log, http.StatusServiceUnavailable)
		rec := httptest.NewRecorder()
		req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/ready?plugin=http", nil)
		h.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)
		reports := parseReports(t, rec.Body.Bytes())
		require.Len(t, reports, 1)
		assert.Equal(t, "http", reports[0].PluginName)
		assert.Equal(t, 200, reports[0].StatusCode)
	})

	t.Run("Filtered_NonExistent", func(t *testing.T) {
		registry := map[string]Readiness{
			"http": &mockReadiness{name: "http", st: &apiStatus.Status{Code: 200}},
		}
		h := NewReadyHandler(registry, newShutdownPtr(false), log, http.StatusServiceUnavailable)
		rec := httptest.NewRecorder()
		req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/ready?plugin=nonexistent", nil)
		h.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)
		reports := parseReports(t, rec.Body.Bytes())
		assert.Empty(t, reports)
	})

	t.Run("Filtered_Error", func(t *testing.T) {
		registry := map[string]Readiness{
			"http": &mockReadiness{name: "http", err: errors.New("not ready")},
		}
		h := NewReadyHandler(registry, newShutdownPtr(false), log, http.StatusServiceUnavailable)
		rec := httptest.NewRecorder()
		req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/ready?plugin=http", nil)
		h.ServeHTTP(rec, req)

		// Filtered path DOES call WriteHeader on error (opposite of health filtered path)
		assert.Equal(t, http.StatusServiceUnavailable, rec.Code)
		reports := parseReports(t, rec.Body.Bytes())
		require.Len(t, reports, 1)
		assert.Equal(t, "not ready", reports[0].ErrorMessage)
		assert.Equal(t, http.StatusInternalServerError, reports[0].StatusCode)
	})

	t.Run("Filtered_NilStatus", func(t *testing.T) {
		registry := map[string]Readiness{
			"http": &mockReadiness{name: "http", st: nil, err: nil},
		}
		h := NewReadyHandler(registry, newShutdownPtr(false), log, http.StatusServiceUnavailable)
		rec := httptest.NewRecorder()
		req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/ready?plugin=http", nil)
		h.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)
		reports := parseReports(t, rec.Body.Bytes())
		require.Len(t, reports, 1)
		assert.Equal(t, "plugin is not available", reports[0].ErrorMessage)
		assert.Equal(t, http.StatusServiceUnavailable, reports[0].StatusCode)
	})

	t.Run("Filtered_ServerError", func(t *testing.T) {
		registry := map[string]Readiness{
			"http": &mockReadiness{name: "http", st: &apiStatus.Status{Code: 500}},
		}
		h := NewReadyHandler(registry, newShutdownPtr(false), log, http.StatusServiceUnavailable)
		rec := httptest.NewRecorder()
		req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/ready?plugin=http", nil)
		h.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusServiceUnavailable, rec.Code)
		reports := parseReports(t, rec.Body.Bytes())
		require.Len(t, reports, 1)
		assert.Equal(t, "internal server error, see logs", reports[0].ErrorMessage)
		assert.Equal(t, http.StatusServiceUnavailable, reports[0].StatusCode)
	})

	t.Run("Filtered_UnexpectedCode", func(t *testing.T) {
		registry := map[string]Readiness{
			"http": &mockReadiness{name: "http", st: &apiStatus.Status{Code: 450}},
		}
		h := NewReadyHandler(registry, newShutdownPtr(false), log, http.StatusServiceUnavailable)
		rec := httptest.NewRecorder()
		req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/ready?plugin=http", nil)
		h.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)
		reports := parseReports(t, rec.Body.Bytes())
		require.Len(t, reports, 1)
		assert.Equal(t, "unexpected status code", reports[0].ErrorMessage)
		assert.Equal(t, 450, reports[0].StatusCode)
	})

	// ---- Custom unavailable status code ----

	t.Run("CustomUnavailableCode", func(t *testing.T) {
		registry := map[string]Readiness{
			"http": &mockReadiness{name: "http", st: &apiStatus.Status{Code: 500}},
		}
		h := NewReadyHandler(registry, newShutdownPtr(false), log, http.StatusInternalServerError)
		rec := httptest.NewRecorder()
		req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/ready", nil)
		h.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusInternalServerError, rec.Code)
		reports := parseReports(t, rec.Body.Bytes())
		require.Len(t, reports, 1)
		assert.Equal(t, http.StatusInternalServerError, reports[0].StatusCode)
	})
}

// --- Jobs Handler Tests ---

func TestJobsHandler(t *testing.T) {
	log := zap.NewNop()

	t.Run("Shutdown", func(t *testing.T) {
		h := NewJobsHandler(nil, newShutdownPtr(true), log, http.StatusServiceUnavailable)
		rec := httptest.NewRecorder()
		req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/jobs", nil)
		h.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusServiceUnavailable, rec.Code)
		assert.Contains(t, rec.Body.String(), "service is shutting down")
	})

	t.Run("NilRegistry", func(t *testing.T) {
		h := NewJobsHandler(nil, newShutdownPtr(false), log, http.StatusServiceUnavailable)
		rec := httptest.NewRecorder()
		req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/jobs", nil)
		h.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusServiceUnavailable, rec.Code)
		assert.Contains(t, rec.Body.String(), "jobs plugin not found")
	})

	t.Run("JobsStateError", func(t *testing.T) {
		jc := &mockJobsChecker{err: errors.New("state error")}
		h := NewJobsHandler(jc, newShutdownPtr(false), log, http.StatusServiceUnavailable)
		rec := httptest.NewRecorder()
		req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/jobs", nil)
		h.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusServiceUnavailable, rec.Code)
		assert.Contains(t, rec.Body.String(), "jobs plugin not found")
	})

	t.Run("EmptyState", func(t *testing.T) {
		jc := &mockJobsChecker{states: []*jobsApi.State{}}
		h := NewJobsHandler(jc, newShutdownPtr(false), log, http.StatusServiceUnavailable)
		rec := httptest.NewRecorder()
		req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/jobs", nil)
		h.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)
		reports := parseJobsReports(t, rec.Body.Bytes())
		assert.Empty(t, reports)
	})

	t.Run("MultipleJobs", func(t *testing.T) {
		jc := &mockJobsChecker{
			states: []*jobsApi.State{
				{Pipeline: "pipe1", Driver: "memory", Priority: 10, Ready: true, Queue: "default", Active: 5, Delayed: 1, Reserved: 2},
				{Pipeline: "pipe2", Driver: "memory", Priority: 20, Ready: false, Queue: "high", Active: 0, Delayed: 3, Reserved: 0, ErrorMessage: "paused"},
			},
		}
		h := NewJobsHandler(jc, newShutdownPtr(false), log, http.StatusServiceUnavailable)
		rec := httptest.NewRecorder()
		req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/jobs", nil)
		h.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)
		reports := parseJobsReports(t, rec.Body.Bytes())
		require.Len(t, reports, 2)

		assert.Equal(t, "pipe1", reports[0].Pipeline)
		assert.Equal(t, "memory", reports[0].Driver)
		assert.Equal(t, uint64(10), reports[0].Priority)
		assert.True(t, reports[0].Ready)
		assert.Equal(t, int64(5), reports[0].Active)
		assert.Equal(t, int64(1), reports[0].Delayed)
		assert.Equal(t, int64(2), reports[0].Reserved)

		assert.Equal(t, "pipe2", reports[1].Pipeline)
		assert.False(t, reports[1].Ready)
		assert.Equal(t, "paused", reports[1].ErrorMessage)
	})
}

// --- Fuzz Tests ---

func FuzzHealthPluginQuery(f *testing.F) {
	f.Add("http")
	f.Add("")
	f.Add("nonexistent")
	f.Add("a&plugin=b")

	registry := map[string]Checker{
		"http": &mockChecker{name: "http", st: &apiStatus.Status{Code: 200}},
	}
	log := zap.NewNop()
	h := NewHealthHandler(registry, newShutdownPtr(false), log, http.StatusServiceUnavailable)

	f.Fuzz(func(t *testing.T, query string) {
		rec := httptest.NewRecorder()
		target := "/health?" + url.Values{"plugin": {query}}.Encode()
		req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, target, nil)
		h.ServeHTTP(rec, req)

		if rec.Code < 100 || rec.Code >= 600 {
			t.Errorf("unexpected HTTP status code: %d", rec.Code)
		}
	})
}

func FuzzReadyPluginQuery(f *testing.F) {
	f.Add("http")
	f.Add("")
	f.Add("nonexistent")
	f.Add("a&plugin=b")

	registry := map[string]Readiness{
		"http": &mockReadiness{name: "http", st: &apiStatus.Status{Code: 200}},
	}
	log := zap.NewNop()
	h := NewReadyHandler(registry, newShutdownPtr(false), log, http.StatusServiceUnavailable)

	f.Fuzz(func(t *testing.T, query string) {
		rec := httptest.NewRecorder()
		target := "/ready?" + url.Values{"plugin": {query}}.Encode()
		req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, target, nil)
		h.ServeHTTP(rec, req)

		if rec.Code < 100 || rec.Code >= 600 {
			t.Errorf("unexpected HTTP status code: %d", rec.Code)
		}
	})
}
