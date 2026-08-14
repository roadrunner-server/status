package tests

import (
	"net/http"
	"testing"

	"tests/helpers"

	statusV1 "github.com/roadrunner-server/api-go/v6/status/v1"
	"github.com/roadrunner-server/jobs/v6"
	"github.com/roadrunner-server/memory/v6"
	rpcPlugin "github.com/roadrunner-server/rpc/v6"
	"github.com/roadrunner-server/server/v6"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	jobsCfg  = "configs/.rr-jobs-status.yaml"
	jobsAddr = "127.0.0.1:35544"
	jobsURL  = "http://" + jobsAddr
	jobsRPC  = "127.0.0.1:6007"
)

// jobsPlugins is the stack the two memory pipelines of the jobs config need.
func jobsPlugins(t *testing.T) []any {
	t.Helper()

	return []any{
		&rpcPlugin.Plugin{},
		&server.Plugin{},
		&jobs.Plugin{},
		&memory.Plugin{},
		helpers.NewStatusPlugin(t),
	}
}

// TestJobsStatus checks that /jobs reports both pipelines of the config with
// the priority they were declared with and an empty queue.
func TestJobsStatus(t *testing.T) {
	helpers.Start(t, jobsCfg, jobsPlugins(t), helpers.WithTCPProbe(jobsAddr))

	code, reports := helpers.GetJobsReports(t, jobsURL+"/jobs")
	assert.Equal(t, http.StatusOK, code)
	require.Len(t, reports, 2)

	for _, report := range reports {
		assert.Equal(t, uint64(13), report.Priority)
		assert.True(t, report.Ready)
		assert.Equal(t, int64(0), report.Active)
		assert.Equal(t, int64(0), report.Delayed)
		assert.Equal(t, int64(0), report.Reserved)
		assert.Equal(t, "memory", report.Driver)
		assert.Empty(t, report.ErrorMessage)
	}
}

// TestJobsReadiness checks the readiness of the jobs plugin over both the http
// endpoint and the rpc service.
func TestJobsReadiness(t *testing.T) {
	helpers.Start(t, jobsCfg, jobsPlugins(t), helpers.WithTCPProbe(jobsAddr))

	code, reports := helpers.GetReports(t, jobsURL+"/ready?plugin=jobs")
	assert.Equal(t, http.StatusOK, code)

	require.Len(t, reports, 1)
	assert.Equal(t, "jobs", reports[0].PluginName)
	assert.Empty(t, reports[0].ErrorMessage)
	assert.Equal(t, http.StatusOK, reports[0].StatusCode)

	client := helpers.RPC(t, jobsRPC)

	for _, method := range []string{"status.Ready", "status.Status"} {
		t.Run(method, func(t *testing.T) {
			rsp := &statusV1.Response{}
			require.NoError(t, client.Call(method, &statusV1.Request{Plugin: "jobs"}, rsp))
			assert.Equal(t, int64(http.StatusOK), rsp.GetCode())
		})
	}
}
