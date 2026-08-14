package tests

import (
	"net/http"
	"testing"
	"time"

	"tests/helpers"

	"github.com/roadrunner-server/jobs/v6"
	rpcPlugin "github.com/roadrunner-server/rpc/v6"
	"github.com/roadrunner-server/server/v6"
	"github.com/stretchr/testify/assert"
)

const (
	shutdownCfg   = "configs/.rr-status-503.yaml"
	shutdownAddr  = "127.0.0.1:34711"
	shutdownURL   = "http://" + shutdownAddr
	shutdownGrace = time.Second * 10
)

// TestShutdown503 checks the endpoints of a stopped container: Plugin.Stop only
// raises the shutdown flag, so the status listener still answers while the rest
// of the container drains.
func TestShutdown503(t *testing.T) {
	stop := helpers.Start(t, shutdownCfg, []any{
		&rpcPlugin.Plugin{},
		&server.Plugin{},
		&jobs.Plugin{},
		helpers.NewStatusPlugin(t),
	},
		helpers.WithTCPProbe(shutdownAddr),
		helpers.WithGracefulTimeout(shutdownGrace),
		helpers.WithConfigTimeout(shutdownGrace),
	)

	// returns once the container has been stopped
	stop()

	// liveness stays 200 so the orchestrator does not kill the draining process
	code, body := helpers.GetBody(t, shutdownURL+"/health")
	assert.Equal(t, http.StatusOK, code)
	assert.Contains(t, body, "service is shutting down")

	code, body = helpers.GetBody(t, shutdownURL+"/ready")
	assert.Equal(t, http.StatusServiceUnavailable, code)
	assert.Contains(t, body, "service is shutting down")

	code, body = helpers.GetBody(t, shutdownURL+"/jobs")
	assert.Equal(t, http.StatusServiceUnavailable, code)
	assert.Contains(t, body, "service is shutting down")
}
