package helpers

import (
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

const (
	// listenerTimeout caps how long WaitListener waits for an address to accept.
	listenerTimeout = time.Second * 15
	// listenerTick is the interval between two connection attempts.
	listenerTick = time.Millisecond * 20
)

// WaitListener waits until addr accepts a connection. The http plugin binds its
// listener only after the worker pool is allocated, so the readiness of the
// status listener says nothing about it.
func WaitListener(t *testing.T, network, addr string) {
	t.Helper()

	require.Eventually(t, func() bool {
		var d net.Dialer

		conn, err := d.DialContext(t.Context(), network, addr)
		if err != nil {
			return false
		}

		return conn.Close() == nil
	}, listenerTimeout, listenerTick, "listener %s did not start", addr)
}
