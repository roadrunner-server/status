package tests

import (
	"net/http"
	"testing"

	"tests/helpers"

	statusV1 "github.com/roadrunner-server/api-go/v6/status/v1"
	httpPlugin "github.com/roadrunner-server/http/v6"
	rpcPlugin "github.com/roadrunner-server/rpc/v6"
	"github.com/roadrunner-server/server/v6"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestStatusRPC asks the status rpc service for the state of a running plugin.
func TestStatusRPC(t *testing.T) {
	helpers.Start(t, statusInitCfg, []any{
		&rpcPlugin.Plugin{},
		&server.Plugin{},
		&httpPlugin.Plugin{},
		helpers.NewStatusPlugin(t),
	}, helpers.WithTCPProbe(statusInitAddr))

	helpers.WaitListener(t, "tcp", sharedHTTPAddr)

	rsp := &statusV1.Response{}
	require.NoError(t, helpers.RPC(t, statusInitRPC).Call("status.Status", &statusV1.Request{Plugin: "http"}, rsp))
	assert.Equal(t, int64(http.StatusOK), rsp.GetCode())
}

// TestRPCUnknownPlugin runs the minimal stack the rpc service needs: no
// Checker or Readiness provider is registered, so every name is unknown and the
// test needs no PHP worker.
func TestRPCUnknownPlugin(t *testing.T) {
	helpers.Start(t, statusInitCfg, []any{
		&rpcPlugin.Plugin{},
		helpers.NewStatusPlugin(t),
	}, helpers.WithTCPProbe(statusInitAddr))

	client := helpers.RPC(t, statusInitRPC)

	for _, method := range []string{"status.Status", "status.Ready"} {
		t.Run(method, func(t *testing.T) {
			err := client.Call(method, &statusV1.Request{Plugin: "nonexistent"}, &statusV1.Response{})
			require.Error(t, err)
			assert.Contains(t, err.Error(), "no such plugin")
		})
	}
}
