package helpers

import (
	"encoding/json"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/roadrunner-server/status/v6"
	"github.com/stretchr/testify/require"
)

// requestTimeout bounds a single request, so a listener that accepts the
// connection but never answers fails the test instead of hanging until the
// go test timeout.
const requestTimeout = time.Second * 10

// GetBody issues a GET and returns the status code and the body. The body is
// read and closed before returning.
func GetBody(t *testing.T, url string) (int, string) {
	t.Helper()

	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, url, nil)
	require.NoError(t, err)

	client := &http.Client{Timeout: requestTimeout}

	r, err := client.Do(req)
	require.NoError(t, err)

	defer func() {
		_ = r.Body.Close()
	}()

	b, err := io.ReadAll(r.Body)
	require.NoError(t, err)

	return r.StatusCode, string(b)
}

// GetReports issues a GET against /health or /ready and decodes the plugin reports.
func GetReports(t *testing.T, url string) (int, []*status.Report) {
	t.Helper()

	return getJSONList[status.Report](t, url)
}

// GetJobsReports issues a GET against /jobs and decodes the pipeline reports.
func GetJobsReports(t *testing.T, url string) (int, []*status.JobsReport) {
	t.Helper()

	return getJSONList[status.JobsReport](t, url)
}

func getJSONList[T any](t *testing.T, url string) (int, []*T) {
	t.Helper()

	code, body := GetBody(t, url)

	var list []*T
	require.NoError(t, json.Unmarshal([]byte(body), &list), "body: %s", body)

	return code, list
}
