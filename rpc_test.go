package status

import (
	stderr "errors"
	"fmt"
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
)

func TestConnectCodeFor(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want connect.Code
	}{
		{name: "plugin not found", err: errPluginNotFound, want: connect.CodeNotFound},
		// status()/ready() wrap the sentinel as fmt.Errorf("%w: %s", errPluginNotFound, name),
		// so the mapping must still resolve through errors.Is unwrapping.
		{name: "wrapped plugin not found", err: fmt.Errorf("%w: http", errPluginNotFound), want: connect.CodeNotFound},
		{name: "any other error", err: stderr.New("boom"), want: connect.CodeInternal},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, connectCodeFor(tt.err))
		})
	}
}
