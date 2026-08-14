package status

import (
	stderr "errors"
	"log/slog"
	"net"
	"testing"
	"time"

	"github.com/roadrunner-server/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// initConfigurer is a minimal Configurer for exercising Plugin.Init without
// standing up a full container.
type initConfigurer struct {
	cfg          *Config
	unmarshalErr error
	has          bool
}

func (c *initConfigurer) Has(string) bool { return c.has }

func (c *initConfigurer) UnmarshalKey(_ string, out any) error {
	if c.unmarshalErr != nil {
		return c.unmarshalErr
	}

	// the plugin passes a **Config, which the config plugin fills in
	dst, ok := out.(**Config)
	if !ok {
		return stderr.New("unexpected destination type")
	}

	cfg := c.cfg
	if cfg == nil {
		cfg = &Config{}
	}

	*dst = cfg

	return nil
}

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

// TestPluginServeAddressTaken checks that a listener the plugin cannot open is
// reported on the channel Serve returns.
func TestPluginServeAddressTaken(t *testing.T) {
	var lc net.ListenConfig

	ln, err := lc.Listen(t.Context(), "tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { _ = ln.Close() })

	p := &Plugin{}
	require.NoError(t, p.Init(&initConfigurer{has: true, cfg: &Config{Address: ln.Addr().String()}}, initLogger{}))

	errCh := p.Serve()
	t.Cleanup(p.StopHTTPServer)

	select {
	case serveErr := <-errCh:
		require.Error(t, serveErr)
	case <-time.After(time.Second * 10):
		t.Fatal("the address is taken, but the plugin reported no error")
	}
}

// TestPluginUnknownPlugin pins the sentinel both lookups wrap, which the rpc
// service turns into the "no such plugin" reply.
func TestPluginUnknownPlugin(t *testing.T) {
	p := &Plugin{}
	require.NoError(t, p.Init(&initConfigurer{has: true}, initLogger{}))

	_, err := p.status("nonexistent")
	require.ErrorIs(t, err, errPluginNotFound)

	_, err = p.ready("nonexistent")
	require.ErrorIs(t, err, errPluginNotFound)
}
