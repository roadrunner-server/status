package status

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"testing"
	"time"

	"connectrpc.com/connect"
	statusV2 "github.com/roadrunner-server/api-go/v6/status/v2"
	"github.com/roadrunner-server/api-go/v6/status/v2/statusV2connect"
	"github.com/roadrunner-server/config/v6"
	"github.com/roadrunner-server/endure/v2"
	httpPlugin "github.com/roadrunner-server/http/v6"
	"github.com/roadrunner-server/jobs/v6"
	"github.com/roadrunner-server/logger/v6"
	"github.com/roadrunner-server/memory/v6"
	rpcPlugin "github.com/roadrunner-server/rpc/v6"
	"github.com/roadrunner-server/server/v6"
	"github.com/roadrunner-server/status/v6"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/net/http2"
)

// newStatusClient builds an h2c Connect client for the migrated
// status.v2.StatusService served by the rpc plugin.
func newStatusClient(t *testing.T, address string) statusV2connect.StatusServiceClient {
	t.Helper()
	httpc := &http.Client{
		Timeout: 10 * time.Second,
		Transport: &http2.Transport{
			AllowHTTP: true,
			DialTLSContext: func(ctx context.Context, network, addr string, _ *tls.Config) (net.Conn, error) {
				return new(net.Dialer).DialContext(ctx, network, addr)
			},
		},
	}
	t.Cleanup(httpc.CloseIdleConnections)
	return statusV2connect.NewStatusServiceClient(httpc, "http://"+address)
}

func TestStatusHttp(t *testing.T) {
	cont := endure.New(slog.LevelDebug, endure.GracefulShutdownTimeout(time.Minute))

	cfg := &config.Plugin{
		Version: "2024.1.0",
		Path:    "configs/.rr-status-init.yaml",
	}

	sp := &status.Plugin{}
	err := cont.RegisterAll(
		cfg,
		&logger.Plugin{},
		&server.Plugin{},
		&httpPlugin.Plugin{},
		sp,
	)
	assert.NoError(t, err)

	err = cont.Init()
	if err != nil {
		t.Fatal(err)
	}

	ch, err := cont.Serve()
	assert.NoError(t, err)

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)

	wg := &sync.WaitGroup{}
	wg.Add(1)

	stopCh := make(chan struct{}, 1)

	go func() {
		defer wg.Done()
		for {
			select {
			case e := <-ch:
				assert.Fail(t, "error", e.Error.Error())
				err = cont.Stop()
				if err != nil {
					assert.FailNow(t, "error", err.Error())
				}
			case <-sig:
				err = cont.Stop()
				if err != nil {
					assert.FailNow(t, "error", err.Error())
				}
				return
			case <-stopCh:
				// timeout
				err = cont.Stop()
				if err != nil {
					assert.FailNow(t, "error", err.Error())
				}
				return
			}
		}
	}()

	time.Sleep(time.Second)
	t.Run("CheckerGetStatus", checkHTTPStatus)
	t.Run("CheckerGetStatusAll", checkHTTPStatusAll)
	t.Run("JobsEndpointWithoutPlugin", func(t *testing.T) {
		req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "http://127.0.0.1:34333/jobs", nil)
		assert.NoError(t, err)

		r, err := http.DefaultClient.Do(req)
		assert.NoError(t, err)
		require.NotNil(t, r)

		b, err := io.ReadAll(r.Body)
		assert.NoError(t, err)
		assert.Equal(t, http.StatusServiceUnavailable, r.StatusCode)
		assert.Contains(t, string(b), "jobs plugin not found")

		err = r.Body.Close()
		assert.NoError(t, err)
	})

	stopCh <- struct{}{}
	wg.Wait()

	t.Cleanup(func() {
		sp.StopHTTPServer()
	})
}

func TestStatusRPC(t *testing.T) {
	cont := endure.New(slog.LevelDebug, endure.GracefulShutdownTimeout(time.Minute))

	cfg := &config.Plugin{
		Version: "2024.1.0",
		Path:    "configs/.rr-status-init.yaml",
	}

	sp := &status.Plugin{}
	err := cont.RegisterAll(
		cfg,
		&rpcPlugin.Plugin{},
		&logger.Plugin{},
		&server.Plugin{},
		sp,
		&httpPlugin.Plugin{},
	)
	assert.NoError(t, err)

	err = cont.Init()
	if err != nil {
		t.Fatal(err)
	}

	ch, err := cont.Serve()
	assert.NoError(t, err)

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)

	wg := &sync.WaitGroup{}
	wg.Add(1)

	stopCh := make(chan struct{}, 1)

	go func() {
		defer wg.Done()
		for {
			select {
			case e := <-ch:
				assert.Fail(t, "error", e.Error.Error())
				err = cont.Stop()
				if err != nil {
					assert.FailNow(t, "error", err.Error())
				}
			case <-sig:
				err = cont.Stop()
				if err != nil {
					assert.FailNow(t, "error", err.Error())
				}
				return
			case <-stopCh:
				// timeout
				err = cont.Stop()
				if err != nil {
					assert.FailNow(t, "error", err.Error())
				}
				return
			}
		}
	}()

	time.Sleep(time.Second)
	t.Run("CheckerGetStatusRpc", func(t *testing.T) {
		checkRPCStatus(t, "http", 200, "6005")
	})
	stopCh <- struct{}{}
	wg.Wait()

	t.Cleanup(func() {
		sp.StopHTTPServer()
	})
}

func TestReadyHttp(t *testing.T) {
	cont := endure.New(slog.LevelDebug, endure.GracefulShutdownTimeout(time.Minute))

	cfg := &config.Plugin{
		Version: "2024.1.0",
		Path:    "configs/.rr-status-init.yaml",
	}

	sp := &status.Plugin{}
	err := cont.RegisterAll(
		cfg,
		&logger.Plugin{},
		&server.Plugin{},
		&httpPlugin.Plugin{},
		sp,
	)
	assert.NoError(t, err)

	err = cont.Init()
	if err != nil {
		t.Fatal(err)
	}

	ch, err := cont.Serve()
	assert.NoError(t, err)

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)

	wg := &sync.WaitGroup{}
	wg.Add(1)

	stopCh := make(chan struct{}, 1)

	go func() {
		defer wg.Done()
		for {
			select {
			case e := <-ch:
				assert.Fail(t, "error", e.Error.Error())
				err = cont.Stop()
				if err != nil {
					assert.FailNow(t, "error", err.Error())
				}
			case <-sig:
				err = cont.Stop()
				if err != nil {
					assert.FailNow(t, "error", err.Error())
				}
				return
			case <-stopCh:
				// timeout
				err = cont.Stop()
				if err != nil {
					assert.FailNow(t, "error", err.Error())
				}
				return
			}
		}
	}()

	time.Sleep(time.Second)
	t.Run("CheckerGetReadiness", checkHTTPReadiness)
	t.Run("CheckerGetReadinessAll", checkHTTPReadinessAll)

	stopCh <- struct{}{}
	wg.Wait()

	t.Cleanup(func() {
		sp.StopHTTPServer()
	})
}

func TestReadinessRPCWorkerNotReady(t *testing.T) {
	cont := endure.New(slog.LevelDebug, endure.GracefulShutdownTimeout(time.Minute))

	cfg := &config.Plugin{
		Version: "2024.1.0",
		Path:    "configs/.rr-ready-init.yaml",
	}

	sp := &status.Plugin{}
	err := cont.RegisterAll(
		cfg,
		&rpcPlugin.Plugin{},
		&logger.Plugin{},
		&server.Plugin{},
		&httpPlugin.Plugin{},
		sp,
	)
	assert.NoError(t, err)

	err = cont.Init()
	if err != nil {
		t.Fatal(err)
	}

	ch, err := cont.Serve()
	assert.NoError(t, err)

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)

	wg := &sync.WaitGroup{}
	wg.Add(1)

	stopCh := make(chan struct{}, 1)

	go func() {
		defer wg.Done()
		for {
			select {
			case e := <-ch:
				assert.Fail(t, "error", e.Error.Error())
				err = cont.Stop()
				if err != nil {
					assert.FailNow(t, "error", err.Error())
				}
			case <-sig:
				err = cont.Stop()
				if err != nil {
					assert.FailNow(t, "error", err.Error())
				}
				return
			case <-stopCh:
				// timeout, error here is OK, because in the PHP we are sleeping for the 300s
				_ = cont.Stop()
				return
			}
		}
	}()

	time.Sleep(time.Second)
	t.Run("DoHttpReq", doHTTPReq)
	time.Sleep(time.Second * 5)
	t.Run("CheckerGetReadiness2", checkHTTPReadiness2)
	t.Run("CheckerGetRpcReadiness", func(t *testing.T) {
		checkRPCReadiness(t, "http", 503, "6006")
	})
	stopCh <- struct{}{}
	wg.Wait()

	t.Cleanup(func() {
		sp.StopHTTPServer()
	})
}

func TestJobsStatus(t *testing.T) {
	cont := endure.New(slog.LevelDebug, endure.GracefulShutdownTimeout(time.Minute))

	cfg := &config.Plugin{
		Version: "2023.1.0",
		Path:    "configs/.rr-jobs-status.yaml",
	}

	sp := &status.Plugin{}
	err := cont.RegisterAll(
		cfg,
		&rpcPlugin.Plugin{},
		&logger.Plugin{},
		&server.Plugin{},
		sp,
		&jobs.Plugin{},
		&memory.Plugin{},
	)
	assert.NoError(t, err)

	err = cont.Init()
	require.NoError(t, err)

	ch, err := cont.Serve()
	assert.NoError(t, err)

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)

	wg := &sync.WaitGroup{}
	wg.Add(1)

	stopCh := make(chan struct{}, 1)

	go func() {
		defer wg.Done()
		for {
			select {
			case e := <-ch:
				assert.Fail(t, "error", e.Error.Error())
				err = cont.Stop()
				if err != nil {
					assert.FailNow(t, "error", err.Error())
				}
			case <-sig:
				err = cont.Stop()
				if err != nil {
					assert.FailNow(t, "error", err.Error())
				}
				return
			case <-stopCh:
				// timeout, error here is OK, because in the PHP we are sleeping for the 300s
				_ = cont.Stop()
				return
			}
		}
	}()

	time.Sleep(time.Second)

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "http://127.0.0.1:35544/jobs", nil)
	assert.NoError(t, err)

	r, err := http.DefaultClient.Do(req)
	assert.NoError(t, err)
	require.NotNil(t, r)

	b, err := io.ReadAll(r.Body)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, r.StatusCode)

	jr := make([]*status.JobsReport, 0, 2)

	err = json.Unmarshal(b, &jr)
	require.NoError(t, err)

	require.Len(t, jr, 2)
	require.Equal(t, jr[0].Priority, uint64(13))
	require.Equal(t, jr[0].Ready, true)
	require.Equal(t, jr[0].Active, int64(0))
	require.Equal(t, jr[0].Delayed, int64(0))
	require.Equal(t, jr[0].Reserved, int64(0))
	require.Equal(t, jr[0].Driver, "memory")
	require.Equal(t, jr[0].ErrorMessage, "")

	require.Equal(t, jr[1].Priority, uint64(13))
	require.Equal(t, jr[1].Ready, true)
	require.Equal(t, jr[1].Active, int64(0))
	require.Equal(t, jr[1].Delayed, int64(0))
	require.Equal(t, jr[1].Reserved, int64(0))
	require.Equal(t, jr[1].Driver, "memory")
	require.Equal(t, jr[1].ErrorMessage, "")

	err = r.Body.Close()
	assert.NoError(t, err)

	stopCh <- struct{}{}
	wg.Wait()

	t.Cleanup(func() {
		sp.StopHTTPServer()
	})
}

func TestJobsReadiness(t *testing.T) {
	cont := endure.New(slog.LevelDebug, endure.GracefulShutdownTimeout(time.Minute))

	cfg := &config.Plugin{
		Version: "2023.2.0",
		Path:    "configs/.rr-jobs-status.yaml",
	}

	sp := &status.Plugin{}
	err := cont.RegisterAll(
		cfg,
		&rpcPlugin.Plugin{},
		&logger.Plugin{},
		&server.Plugin{},
		sp,
		&jobs.Plugin{},
		&memory.Plugin{},
	)
	assert.NoError(t, err)

	err = cont.Init()
	require.NoError(t, err)

	ch, err := cont.Serve()
	assert.NoError(t, err)

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)

	wg := &sync.WaitGroup{}
	wg.Add(1)

	stopCh := make(chan struct{}, 1)

	go func() {
		defer wg.Done()
		for {
			select {
			case e := <-ch:
				assert.Fail(t, "error", e.Error.Error())
				err = cont.Stop()
				if err != nil {
					assert.FailNow(t, "error", err.Error())
				}
			case <-sig:
				err = cont.Stop()
				if err != nil {
					assert.FailNow(t, "error", err.Error())
				}
				return
			case <-stopCh:
				// timeout, error here is OK, because in the PHP we are sleeping for the 300s
				_ = cont.Stop()
				return
			}
		}
	}()

	time.Sleep(time.Second)
	t.Run("checkJobsReadiness", checkJobsReadiness)
	t.Run("checkJobsRPC", func(t *testing.T) {
		checkRPCReadiness(t, "jobs", 200, "6007")
		checkRPCStatus(t, "jobs", 200, "6007")
	})

	stopCh <- struct{}{}
	wg.Wait()

	t.Cleanup(func() {
		sp.StopHTTPServer()
	})
}

func TestShutdown503(t *testing.T) {
	cont := endure.New(slog.LevelDebug, endure.GracefulShutdownTimeout(time.Second*10))

	cfg := &config.Plugin{
		Version: "2024.1.0",
		Timeout: time.Second * 10,
		Path:    "configs/.rr-status-503.yaml",
	}

	sp := &status.Plugin{}
	err := cont.RegisterAll(
		cfg,
		&rpcPlugin.Plugin{},
		&logger.Plugin{},
		&server.Plugin{},
		sp,
		&jobs.Plugin{},
	)
	assert.NoError(t, err)

	err = cont.Init()
	if err != nil {
		t.Fatal(err)
	}

	ch, err := cont.Serve()
	assert.NoError(t, err)

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)

	wg := &sync.WaitGroup{}
	wg.Add(1)

	stopCh := make(chan struct{}, 1)

	go func() {
		defer wg.Done()
		for {
			select {
			case e := <-ch:
				assert.Fail(t, "error", e.Error.Error())
				err = cont.Stop()
				if err != nil {
					assert.FailNow(t, "error", err.Error())
				}
			case <-sig:
				err = cont.Stop()
				if err != nil {
					assert.FailNow(t, "error", err.Error())
				}
				return
			case <-stopCh:
				// timeout, error here is OK, because in the PHP we are sleeping for the 300s
				_ = cont.Stop()
				return
			}
		}
	}()

	time.Sleep(time.Second)
	go func() {
		httpClient := &http.Client{
			Timeout: time.Second * 10,
		}

		req, err2 := http.NewRequestWithContext(context.Background(), http.MethodGet, "http://127.0.0.1:11934", nil)
		assert.NoError(t, err2)
		rsp, _ := httpClient.Do(req)
		if rsp != nil {
			_ = rsp.Body.Close()
		}
	}()

	time.Sleep(time.Second)
	stopCh <- struct{}{}

	httpClient := &http.Client{
		Timeout: time.Second * 10,
	}

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "http://127.0.0.1:34711/health", nil)
	assert.NoError(t, err)
	require.NotNil(t, req)

	rsp, err := httpClient.Do(req)
	require.NoError(t, err)

	assert.Equal(t, http.StatusOK, rsp.StatusCode)

	req, err = http.NewRequestWithContext(context.Background(), http.MethodGet, "http://127.0.0.1:34711/ready", nil)
	assert.NoError(t, err)
	require.NotNil(t, req)

	rsp, err = httpClient.Do(req)
	require.NoError(t, err)
	require.NotNil(t, rsp)

	assert.Equal(t, http.StatusServiceUnavailable, rsp.StatusCode)
	_ = rsp.Body.Close()

	// Also verify /jobs returns 503 during shutdown
	req, err = http.NewRequestWithContext(context.Background(), http.MethodGet, "http://127.0.0.1:34711/jobs", nil)
	assert.NoError(t, err)
	require.NotNil(t, req)

	rsp, err = httpClient.Do(req)
	require.NoError(t, err)

	b, err := io.ReadAll(rsp.Body)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusServiceUnavailable, rsp.StatusCode)
	assert.Contains(t, string(b), "service is shutting down")

	wg.Wait()

	t.Cleanup(func() {
		if rsp != nil {
			_ = rsp.Body.Close()
		}

		sp.StopHTTPServer()
	})
}

func TestRPCNonExistentPlugin(t *testing.T) {
	cont := endure.New(slog.LevelDebug, endure.GracefulShutdownTimeout(time.Minute))

	cfg := &config.Plugin{
		Version: "2024.1.0",
		Path:    "configs/.rr-status-init.yaml",
	}

	// Minimal stack: this test only verifies the "no such plugin" path of the
	// status RPC handler, so no Checker/Readiness providers are needed and
	// http/server are dropped to keep the test PHP-independent.
	sp := &status.Plugin{}
	err := cont.RegisterAll(
		cfg,
		&rpcPlugin.Plugin{},
		&logger.Plugin{},
		sp,
	)
	assert.NoError(t, err)

	err = cont.Init()
	if err != nil {
		t.Fatal(err)
	}

	ch, err := cont.Serve()
	assert.NoError(t, err)

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)

	wg := &sync.WaitGroup{}
	wg.Add(1)

	stopCh := make(chan struct{}, 1)

	go func() {
		defer wg.Done()
		for {
			select {
			case e := <-ch:
				assert.Fail(t, "error", e.Error.Error())
				err = cont.Stop()
				if err != nil {
					assert.FailNow(t, "error", err.Error())
				}
			case <-sig:
				err = cont.Stop()
				if err != nil {
					assert.FailNow(t, "error", err.Error())
				}
				return
			case <-stopCh:
				err = cont.Stop()
				if err != nil {
					assert.FailNow(t, "error", err.Error())
				}
				return
			}
		}
	}()

	time.Sleep(time.Second)

	t.Run("StatusNonExistent", func(t *testing.T) {
		client := newStatusClient(t, "127.0.0.1:6005")
		_, err := client.Status(t.Context(), connect.NewRequest(&statusV2.StatusRequest{Plugin: "nonexistent"}))
		require.Error(t, err)
		assert.Equal(t, connect.CodeNotFound, connect.CodeOf(err))
	})

	t.Run("ReadyNonExistent", func(t *testing.T) {
		client := newStatusClient(t, "127.0.0.1:6005")
		_, err := client.Ready(t.Context(), connect.NewRequest(&statusV2.StatusRequest{Plugin: "nonexistent"}))
		require.Error(t, err)
		assert.Equal(t, connect.CodeNotFound, connect.CodeOf(err))
	})

	stopCh <- struct{}{}
	wg.Wait()

	t.Cleanup(func() {
		sp.StopHTTPServer()
	})
}

func checkJobsReadiness(t *testing.T) {
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, "http://127.0.0.1:35544/ready?plugin=jobs", nil)
	assert.NoError(t, err)

	r, err := http.DefaultClient.Do(req)
	assert.NoError(t, err)
	b, err := io.ReadAll(r.Body)
	assert.NoError(t, err)

	rep := make([]*status.Report, 0, 2)
	err = json.Unmarshal(b, &rep)
	require.NoError(t, err)

	assert.Equal(t, 200, r.StatusCode)

	assert.Len(t, rep, 1)
	assert.Equal(t, "jobs", rep[0].PluginName)
	assert.Equal(t, "", rep[0].ErrorMessage)
	assert.Equal(t, 200, rep[0].StatusCode)

	err = r.Body.Close()
	assert.NoError(t, err)
}

func checkHTTPStatus(t *testing.T) {
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, "http://127.0.0.1:34333/health?plugin=http&plugin=rpc", nil)
	assert.NoError(t, err)

	r, err := http.DefaultClient.Do(req)
	assert.NoError(t, err)
	b, err := io.ReadAll(r.Body)
	assert.NoError(t, err)
	assert.Equal(t, 200, r.StatusCode)

	rep := make([]*status.Report, 0, 2)
	err = json.Unmarshal(b, &rep)
	require.NoError(t, err)

	assert.Len(t, rep, 1)
	assert.Equal(t, "http", rep[0].PluginName)
	assert.Equal(t, "", rep[0].ErrorMessage)
	assert.Equal(t, 200, rep[0].StatusCode)

	err = r.Body.Close()
	assert.NoError(t, err)
}

func checkHTTPStatusAll(t *testing.T) {
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, "http://127.0.0.1:34333/health", nil)
	assert.NoError(t, err)

	r, err := http.DefaultClient.Do(req)
	assert.NoError(t, err)
	b, err := io.ReadAll(r.Body)
	assert.NoError(t, err)
	assert.Equal(t, 200, r.StatusCode)

	rep := make([]*status.Report, 0, 2)
	err = json.Unmarshal(b, &rep)
	require.NoError(t, err)

	assert.Len(t, rep, 1)
	assert.Equal(t, "http", rep[0].PluginName)
	assert.Equal(t, "", rep[0].ErrorMessage)
	assert.Equal(t, 200, rep[0].StatusCode)

	err = r.Body.Close()
	assert.NoError(t, err)
}

func doHTTPReq(t *testing.T) {
	go func() {
		client := &http.Client{
			Timeout: time.Second * 10,
		}
		req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "http://127.0.0.1:11933", nil)
		assert.NoError(t, err)

		_, err = client.Do(req) //nolint:bodyclose
		assert.Error(t, err)
	}()
}

func checkHTTPReadiness2(t *testing.T) {
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, "http://127.0.0.1:34334/ready?plugin=http&plugin=rpc", nil)
	assert.NoError(t, err)

	r, err := http.DefaultClient.Do(req)
	assert.NoError(t, err)
	b, err := io.ReadAll(r.Body)
	require.NoError(t, err)

	res := make([]*status.Report, 0, 2)
	err = json.Unmarshal(b, &res)
	require.NoError(t, err)
	assert.Len(t, res, 1)
	assert.Equal(t, "http", res[0].PluginName)
	assert.Equal(t, "internal server error, see logs", res[0].ErrorMessage)
	assert.Equal(t, 503, res[0].StatusCode)
	assert.Equal(t, 503, r.StatusCode)

	err = r.Body.Close()
	assert.NoError(t, err)
}

func checkHTTPReadinessAll(t *testing.T) {
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, "http://127.0.0.1:34333/ready", nil)
	assert.NoError(t, err)

	r, err := http.DefaultClient.Do(req)
	assert.NoError(t, err)
	b, err := io.ReadAll(r.Body)
	assert.NoError(t, err)
	assert.Equal(t, 200, r.StatusCode)
	res := make([]*status.Report, 0, 2)
	err = json.Unmarshal(b, &res)
	assert.NoError(t, err)

	err = r.Body.Close()
	assert.NoError(t, err)
}

func checkHTTPReadiness(t *testing.T) {
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, "http://127.0.0.1:34333/ready?plugin=http&plugin=rpc", nil)
	assert.NoError(t, err)

	r, err := http.DefaultClient.Do(req)
	assert.NoError(t, err)
	b, err := io.ReadAll(r.Body)
	assert.NoError(t, err)
	assert.Equal(t, 200, r.StatusCode)
	res := make([]*status.Report, 0, 2)
	err = json.Unmarshal(b, &res)
	assert.NoError(t, err)

	err = r.Body.Close()
	assert.NoError(t, err)
}

func checkRPCReadiness(t *testing.T, plugin string, code int64, port string) {
	client := newStatusClient(t, "127.0.0.1:"+port)
	resp, err := client.Ready(t.Context(), connect.NewRequest(&statusV2.StatusRequest{Plugin: plugin}))
	require.NoError(t, err)
	assert.Equal(t, code, resp.Msg.GetCode())
}

func checkRPCStatus(t *testing.T, plugin string, code int64, port string) {
	client := newStatusClient(t, "127.0.0.1:"+port)
	resp, err := client.Status(t.Context(), connect.NewRequest(&statusV2.StatusRequest{Plugin: plugin}))
	require.NoError(t, err)
	assert.Equal(t, code, resp.Msg.GetCode())
}
