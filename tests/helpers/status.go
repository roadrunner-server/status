package helpers

import (
	"testing"

	"github.com/roadrunner-server/status/v6"
)

// NewStatusPlugin returns a status plugin whose http listener is closed by
// t.Cleanup. Plugin.Stop only raises the shutdown flag, so without the explicit
// close the address stays bound and the next test on it fails to listen. The
// cleanup is registered before the container is started, so it runs after the
// container has been stopped.
func NewStatusPlugin(t *testing.T) *status.Plugin {
	t.Helper()

	p := &status.Plugin{}
	t.Cleanup(p.StopHTTPServer)

	return p
}
