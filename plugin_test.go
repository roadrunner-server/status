package status

import (
	stderr "errors"
	"log/slog"
	"testing"

	"github.com/roadrunner-server/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// initConfigurer is a minimal Configurer for exercising Plugin.Init's early
// error returns without standing up a full container.
type initConfigurer struct {
	has          bool
	unmarshalErr error
}

func (c *initConfigurer) Has(string) bool                { return c.has }
func (c *initConfigurer) UnmarshalKey(string, any) error { return c.unmarshalErr }

type initLogger struct{}

func (initLogger) NamedLogger(string) *slog.Logger { return slog.New(slog.DiscardHandler) }

func TestPluginInit(t *testing.T) {
	t.Run("disabled when config section is missing", func(t *testing.T) {
		err := (&Plugin{}).Init(&initConfigurer{has: false}, initLogger{})
		require.Error(t, err)
		assert.True(t, errors.Is(errors.Disabled, err))
	})

	t.Run("disabled on unmarshal error", func(t *testing.T) {
		err := (&Plugin{}).Init(&initConfigurer{has: true, unmarshalErr: stderr.New("bad config")}, initLogger{})
		require.Error(t, err)
		assert.True(t, errors.Is(errors.Disabled, err))
	})
}
