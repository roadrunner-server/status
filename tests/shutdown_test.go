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

	for _, tt := range []struct {
		name     string
		path     string
		wantCode int
	}{
		{name: "Health", path: "/health", wantCode: http.StatusOK},
		{name: "Livez", path: "/livez", wantCode: http.StatusOK},
		{name: "Ready", path: "/ready", wantCode: http.StatusServiceUnavailable},
		{name: "Readyz", path: "/readyz", wantCode: http.StatusServiceUnavailable},
		{name: "Jobs", path: "/jobs", wantCode: http.StatusServiceUnavailable},
	} {
		t.Run(tt.name, func(t *testing.T) {
			code, body := helpers.GetBody(t, shutdownURL+tt.path)
			assert.Equal(t, tt.wantCode, code)
			assert.Contains(t, body, "service is shutting down")
		})
	}
}
