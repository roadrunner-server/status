package helpers

import (
	"context"
	"log/slog"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/roadrunner-server/config/v6"
	"github.com/roadrunner-server/endure/v2"
	"github.com/roadrunner-server/logger/v6"
	"github.com/stretchr/testify/require"
)

const (
	// configVersion is the config schema version used by the test configs.
	configVersion = "2024.1.0"
	// defaultGraceful is the endure graceful shutdown timeout.
	defaultGraceful = time.Minute
	// probeTimeout caps how long Start waits for the server to answer the probe.
	probeTimeout = time.Second * 15
	probeTick    = time.Millisecond * 20
	probeDial    = time.Second
)

// bootCfg holds the options applied to a container before it is started.
type bootCfg struct {
	graceful   time.Duration
	cfgTimeout time.Duration
	probe      func(ctx context.Context) bool
}

// Option customizes the container built by Start.
type Option func(*bootCfg)

// WithGracefulTimeout sets the endure graceful shutdown timeout.
func WithGracefulTimeout(d time.Duration) Option {
	return func(b *bootCfg) { b.graceful = d }
}

// WithConfigTimeout sets the timeout the config plugin hands to the plugins
// that drain on shutdown.
func WithConfigTimeout(d time.Duration) Option {
	return func(b *bootCfg) { b.cfgTimeout = d }
}

// WithTCPProbe makes Start return only once addr accepts a connection.
func WithTCPProbe(addr string) Option {
	return func(b *bootCfg) {
		b.probe = func(ctx context.Context) bool {
			d := net.Dialer{Timeout: probeDial}

			conn, err := d.DialContext(ctx, "tcp", addr)
			if err != nil {
				return false
			}

			return conn.Close() == nil
		}
	}
}

// Start registers the plugins, boots the container and waits for the probe, if
// any, to answer. Errors arriving on the container channel are reported through
// t.Errorf and stop the container, but they do not abort the test.
//
// The returned stop is idempotent and also registered with t.Cleanup, so tests
// asserting on the behavior during shutdown can stop the container mid-test. It
// returns only after cont.Stop has completed.
func Start(t *testing.T, cfgPath string, plugins []any, opts ...Option) func() {
	t.Helper()

	bc := &bootCfg{graceful: defaultGraceful}
	for _, o := range opts {
		o(bc)
	}

	cont := endure.New(slog.LevelDebug, endure.GracefulShutdownTimeout(bc.graceful))
	cfg := &config.Plugin{Version: configVersion, Path: cfgPath, Timeout: bc.cfgTimeout}

	require.NoError(t, cont.RegisterAll(append([]any{cfg, &logger.Plugin{}}, plugins...)...))
	require.NoError(t, cont.Init())

	ch, err := cont.Serve()
	require.NoError(t, err)

	stopCont := sync.OnceValue(cont.Stop)
	done := make(chan struct{})
	wg := &sync.WaitGroup{}

	wg.Go(func() {
		for {
			select {
			case res := <-ch:
				if res == nil {
					return
				}

				t.Errorf("plugin %s reported an error: %v", res.VertexID, res.Error)

				if errS := stopCont(); errS != nil {
					t.Errorf("container stop: %v", errS)
				}
			case <-done:
				if errS := stopCont(); errS != nil {
					t.Errorf("container stop: %v", errS)
				}

				return
			}
		}
	})

	// The drain goroutine calls t.Errorf, so it has to be joined while the test
	// is still running.
	stop := sync.OnceFunc(func() {
		close(done)
		wg.Wait()
	})
	t.Cleanup(stop)

	if bc.probe != nil {
		require.Eventually(t, func() bool { return bc.probe(t.Context()) }, probeTimeout, probeTick, "server did not become ready")
	}

	return stop
}
