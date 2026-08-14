package tests

import (
	"context"
	"net/http"
	"testing"
	"time"

	"tests/helpers"

	statusV1 "github.com/roadrunner-server/api-go/v6/status/v1"
	httpPlugin "github.com/roadrunner-server/http/v6"
	rpcPlugin "github.com/roadrunner-server/rpc/v6"
	"github.com/roadrunner-server/server/v6"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	// sharedHTTPAddr is the http address of both the status-init and the
	// ready-init config, so the tests using them have to run one after another:
	// no t.Parallel here.
	sharedHTTPAddr = "127.0.0.1:11933"

	statusInitCfg  = "configs/.rr-status-init.yaml"
	statusInitAddr = "127.0.0.1:34333"
	statusInitURL  = "http://" + statusInitAddr
	statusInitRPC  = "127.0.0.1:6005"

	readyInitCfg  = "configs/.rr-ready-init.yaml"
	readyInitAddr = "127.0.0.1:34334"
	readyInitURL  = "http://" + readyInitAddr
	readyInitRPC  = "127.0.0.1:6006"
)

// TestStatusEndpoints drives /health, /ready and /jobs against a container
// running the http plugin. The rpc plugin is deliberately left out: the
// ?plugin=http&plugin=rpc queries then prove that a name missing from the
// registry is skipped instead of reported.
func TestStatusEndpoints(t *testing.T) {
	helpers.Start(t, statusInitCfg, []any{
		&server.Plugin{},
		&httpPlugin.Plugin{},
		helpers.NewStatusPlugin(t),
	}, helpers.WithTCPProbe(statusInitAddr))

	// the http plugin binds its listener after the pool is allocated, which is
	// what makes its status and readiness meaningful
	helpers.WaitListener(t, "tcp", sharedHTTPAddr)

	t.Run("HealthFiltered", func(t *testing.T) {
		code, reports := helpers.GetReports(t, statusInitURL+"/health?plugin=http&plugin=rpc")
		assert.Equal(t, http.StatusOK, code)

		require.Len(t, reports, 1)
		assert.Equal(t, "http", reports[0].PluginName)
		assert.Empty(t, reports[0].ErrorMessage)
		assert.Equal(t, http.StatusOK, reports[0].StatusCode)
	})

	t.Run("HealthAll", func(t *testing.T) {
		code, reports := helpers.GetReports(t, statusInitURL+"/health")
		assert.Equal(t, http.StatusOK, code)

		require.Len(t, reports, 1)
		assert.Equal(t, "http", reports[0].PluginName)
		assert.Empty(t, reports[0].ErrorMessage)
		assert.Equal(t, http.StatusOK, reports[0].StatusCode)
	})

	t.Run("ReadyFiltered", func(t *testing.T) {
		code, reports := helpers.GetReports(t, statusInitURL+"/ready?plugin=http&plugin=rpc")
		assert.Equal(t, http.StatusOK, code)

		require.Len(t, reports, 1)
		assert.Equal(t, "http", reports[0].PluginName)
		assert.Empty(t, reports[0].ErrorMessage)
		assert.Equal(t, http.StatusOK, reports[0].StatusCode)
	})

	t.Run("ReadyAll", func(t *testing.T) {
		code, reports := helpers.GetReports(t, statusInitURL+"/ready")
		assert.Equal(t, http.StatusOK, code)

		require.Len(t, reports, 1)
		assert.Equal(t, "http", reports[0].PluginName)
		assert.Empty(t, reports[0].ErrorMessage)
		assert.Equal(t, http.StatusOK, reports[0].StatusCode)
	})

	t.Run("JobsWithoutJobsPlugin", func(t *testing.T) {
		code, body := helpers.GetBody(t, statusInitURL+"/jobs")
		assert.Equal(t, http.StatusServiceUnavailable, code)
		assert.Contains(t, body, "jobs plugin not found")
	})
}

// TestReadinessWorkerBusy occupies the only worker of the pool with a request
// the worker never answers, which leaves the http plugin without a ready
// worker. The rpc assertion lives here rather than in rpc_test.go to avoid a
// second boot of this container.
func TestReadinessWorkerBusy(t *testing.T) {
	helpers.Start(t, readyInitCfg, []any{
		&rpcPlugin.Plugin{},
		&server.Plugin{},
		&httpPlugin.Plugin{},
		helpers.NewStatusPlugin(t),
	}, helpers.WithTCPProbe(readyInitAddr))

	helpers.WaitListener(t, "tcp", sharedHTTPAddr)

	occupyWorker(t, "http://"+sharedHTTPAddr)

	readyURL := readyInitURL + "/ready?plugin=http&plugin=rpc"
	require.Eventually(t, func() bool {
		return statusCode(t.Context(), readyURL) == http.StatusServiceUnavailable
	}, time.Second*15, time.Millisecond*20, "the pool kept a ready worker")

	code, reports := helpers.GetReports(t, readyURL)
	assert.Equal(t, http.StatusServiceUnavailable, code)

	require.Len(t, reports, 1)
	assert.Equal(t, "http", reports[0].PluginName)
	assert.Equal(t, "internal server error, see logs", reports[0].ErrorMessage)
	assert.Equal(t, http.StatusServiceUnavailable, reports[0].StatusCode)

	rsp := &statusV1.Response{}
	require.NoError(t, helpers.RPC(t, readyInitRPC).Call("status.Ready", &statusV1.Request{Plugin: "http"}, rsp))
	assert.Equal(t, int64(http.StatusServiceUnavailable), rsp.GetCode())
}

// occupyWorker sends a request the worker answers only after a sleep longer
// than the test, so the worker stays busy. The request is canceled before the
// container is stopped.
func occupyWorker(t *testing.T, url string) {
	t.Helper()

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	require.NoError(t, err)

	go func() {
		rsp, errR := http.DefaultClient.Do(req)
		if errR == nil {
			_ = rsp.Body.Close()
		}
	}()
}

// statusCode returns the response code of a GET, or -1 if the request failed.
// It reports nothing to the testing framework, so it is safe to call from a
// require.Eventually condition, which runs in its own goroutine.
func statusCode(ctx context.Context, url string) int {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return -1
	}

	rsp, err := http.DefaultClient.Do(req)
	if err != nil {
		return -1
	}

	_ = rsp.Body.Close()

	return rsp.StatusCode
}
