package status

import (
	"log/slog"
	"net/http"
	"sync/atomic"
)

// readiness Handler return 200OK if all Plugins are ready to serve
// if one of the Plugins returns status from the 5xx range, the status for all queries will be 503

type Ready struct {
	log                   *slog.Logger
	unavailableStatusCode int
	statusRegistry        map[string]Readiness
	shutdownInitiated     *atomic.Bool
}

func NewReadyHandler(sr map[string]Readiness, shutdownInitiated *atomic.Bool, log *slog.Logger, usc int) *Ready {
	return &Ready{
		log:                   log,
		statusRegistry:        sr,
		unavailableStatusCode: usc,
		shutdownInitiated:     shutdownInitiated,
	}
}

func (rd *Ready) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if rd.shutdownInitiated != nil && rd.shutdownInitiated.Load() {
		http.Error(w, "service is shutting down", rd.unavailableStatusCode)
		return
	}

	report := make([]*Report, 0, len(rd.statusRegistry))
	unavailable := false

	plg := r.URL.Query()[pluginsQuery]
	if len(plg) == 0 {
		for k, pl := range rd.statusRegistry {
			if pl == nil {
				unavailable = true
				report = append(report, &Report{
					PluginName:   k,
					ErrorMessage: "plugin is nil or not initialized",
					StatusCode:   http.StatusNotFound,
				})
				rd.log.Info("plugin is nil or not initialized", "plugin", k)
				continue
			}

			st, err := pl.Ready()
			rep, failed := evaluatePlugin(k, st, err, rd.unavailableStatusCode)
			unavailable = unavailable || failed
			report = append(report, rep)
		}
	} else {
		for _, name := range plg {
			svc, ok := rd.statusRegistry[name]
			if !ok {
				rd.log.Info("plugin does not support readiness checks", "plugin", name)
				continue
			}

			if svc == nil {
				unavailable = true
				report = append(report, &Report{
					PluginName:   name,
					ErrorMessage: "plugin is nil or not initialized",
					StatusCode:   http.StatusNotFound,
				})
				rd.log.Info("plugin is nil or not initialized", "plugin", name)
				continue
			}

			st, err := svc.Ready()
			rep, failed := evaluatePlugin(name, st, err, rd.unavailableStatusCode)
			unavailable = unavailable || failed
			report = append(report, rep)
		}

		if len(report) == 0 {
			unavailable = true
		}
	}

	writeReport(w, rd.log, report, unavailable, rd.unavailableStatusCode)
}
