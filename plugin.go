package status

import (
	"context"
	"net/http"
	"time"

	jobsApi "github.com/roadrunner-server/api/v4/plugins/v1/jobs"
	"github.com/roadrunner-server/api/v4/plugins/v1/status"
	"github.com/roadrunner-server/endure/v2/dep"
	"github.com/roadrunner-server/errors"
	"go.uber.org/zap"
)

const (
	// PluginName declares public plugin name.
	PluginName        = "status"
	template   string = "plugin: %s | status: %d\n"
)

type Configurer interface {
	// UnmarshalKey takes a single key and unmarshal it into a Struct.
	UnmarshalKey(name string, out any) error
	// Has checks if config section exists.
	Has(name string) bool
}

type Logger interface {
	NamedLogger(name string) *zap.Logger
}

// Checker interface used to get latest status from plugin
type Checker interface {
	Status() (*status.Status, error)
	Name() string
}

type JobsChecker interface {
	JobsState(ctx context.Context) ([]*jobsApi.State, error)
	Name() string
}

// Readiness interface used to get readiness status from the plugin
// that means, that worker poll inside the plugin has 1+ plugins which are ready to work
// at the particular moment
type Readiness interface {
	Ready() (*status.Status, error)
	Name() string
}

type Plugin struct {
	// plugins which needs to be checked just as Status
	statusRegistry map[string]Checker
	// plugins which needs to send Readiness status
	readyRegistry map[string]Readiness
	// jobs plugin checker
	statusJobsRegistry JobsChecker
	server             *http.Server
	log                *zap.Logger
	cfg                *Config
}

func (c *Plugin) Init(cfg Configurer, log Logger) error {
	const op = errors.Op("checker_plugin_init")
	if !cfg.Has(PluginName) {
		return errors.E(op, errors.Disabled)
	}
	err := cfg.UnmarshalKey(PluginName, &c.cfg)
	if err != nil {
		return errors.E(op, errors.Disabled, err)
	}

	// init defaults for the status plugin
	c.cfg.InitDefaults()

	c.readyRegistry = make(map[string]Readiness)
	c.statusRegistry = make(map[string]Checker)

	c.log = log.NamedLogger(PluginName)

	return nil
}

func (c *Plugin) Serve() chan error {
	errCh := make(chan error, 1)

	mux := http.NewServeMux()
	mux.Handle("/health", NewHealthHandler(c.statusRegistry, c.log, c.cfg.UnavailableStatusCode))
	mux.Handle("/ready", NewReadyHandler(c.readyRegistry, c.log, c.cfg.UnavailableStatusCode))
	mux.Handle("/jobs", NewJobsHandler(c.statusJobsRegistry, c.log, c.cfg.UnavailableStatusCode))

	go func() {
		server := &http.Server{
			Addr:                         c.cfg.Address,
			Handler:                      mux,
			DisableGeneralOptionsHandler: false,
			ReadTimeout:                  time.Minute,
			ReadHeaderTimeout:            time.Minute,
			WriteTimeout:                 time.Minute,
			IdleTimeout:                  time.Minute,
		}
		err := server.ListenAndServe()
		if err != nil {
			errCh <- err
		}
	}()

	return errCh
}

func (c *Plugin) Stop(ctx context.Context) error {
	const op = errors.Op("checker_plugin_stop")
	err := c.server.Shutdown(ctx)
	if err != nil {
		return errors.E(op, err)
	}
	return nil
}

// status returns a Checker interface implementation
// Reset named service. This is not an Status interface implementation
func (c *Plugin) status(name string) (*status.Status, error) {
	const op = errors.Op("checker_plugin_status")
	svc, ok := c.statusRegistry[name]
	if !ok {
		return nil, errors.E(op, errors.Errorf("no such plugin: %s", name))
	}

	return svc.Status()
}

// ready used to provide a readiness check for the plugin
func (c *Plugin) ready(name string) (*status.Status, error) {
	const op = errors.Op("checker_plugin_ready")
	svc, ok := c.readyRegistry[name]
	if !ok {
		return nil, errors.E(op, errors.Errorf("no such plugin: %s", name))
	}

	return svc.Ready()
}

// Collects declares services to be collected.
func (c *Plugin) Collects() []*dep.In {
	return []*dep.In{
		dep.Fits(func(p any) {
			r := p.(Readiness)
			c.readyRegistry[r.Name()] = r
		}, (*Readiness)(nil)),
		dep.Fits(func(p any) {
			s := p.(Checker)
			c.statusRegistry[s.Name()] = s
		}, (*Checker)(nil)),
		dep.Fits(func(p any) {
			c.statusJobsRegistry = p.(JobsChecker)
		}, (*JobsChecker)(nil)),
	}
}

// Name of the service.
func (c *Plugin) Name() string {
	return PluginName
}

// RPC returns associated rpc service.
func (c *Plugin) RPC() any {
	return &rpc{srv: c, log: c.log}
}
