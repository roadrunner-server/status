package status

import (
	"encoding/json/jsontext"
	"encoding/json/v2"
	"log/slog"
	"net/http"
	"sync/atomic"

	apiStatus "github.com/roadrunner-server/api-plugins/v6/status"
)

type Health struct {
	log                   *slog.Logger
	unavailableStatusCode int
	statusRegistry        map[string]Checker
	shutdownInitiated     *atomic.Bool
}

func NewHealthHandler(sr map[string]Checker, shutdownInitiated *atomic.Bool, log *slog.Logger, usc int) *Health {
	return &Health{
		statusRegistry:        sr,
		unavailableStatusCode: usc,
		log:                   log,
		shutdownInitiated:     shutdownInitiated,
	}
}

func (rd *Health) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if rd.shutdownInitiated != nil && rd.shutdownInitiated.Load() {
		// Liveness stays 200 during graceful shutdown so the orchestrator does not
		// kill the draining process; readiness (/ready) and /jobs return the
		// configured unavailable code instead. Do NOT collapse onto unavailableStatusCode.
		http.Error(w, "service is shutting down", http.StatusOK)
		return
	}

	report := make([]*Report, 0, len(rd.statusRegistry))
	unavailable := false

	plg := r.URL.Query()[pluginsQuery]
	if len(plg) == 0 {
		rd.log.Debug("no plugins provided, checking all plugins")

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

			st, err := pl.Status()
			rep, failed := evaluatePlugin(k, st, err, rd.unavailableStatusCode)
			unavailable = unavailable || failed
			report = append(report, rep)
		}
	} else {
		for _, name := range plg {
			svc, ok := rd.statusRegistry[name]
			if !ok {
				rd.log.Info("plugin does not support health checks", "plugin", name)
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

			st, err := svc.Status()
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

func evaluatePlugin(name string, st *apiStatus.Status, err error, unavailableCode int) (*Report, bool) {
	if err != nil {
		return &Report{
			PluginName:   name,
			ErrorMessage: err.Error(),
			StatusCode:   unavailableCode,
		}, true
	}

	if st == nil {
		return &Report{
			PluginName:   name,
			ErrorMessage: "plugin is not available",
			StatusCode:   unavailableCode,
		}, true
	}

	switch {
	case st.Code >= 500:
		return &Report{
			PluginName:   name,
			ErrorMessage: "internal server error, see logs",
			StatusCode:   unavailableCode,
		}, true
	case st.Code >= 100 && st.Code <= 400:
		return &Report{
			PluginName: name,
			StatusCode: st.Code,
		}, false
	default:
		return &Report{
			PluginName:   name,
			ErrorMessage: "unexpected status code",
			StatusCode:   st.Code,
		}, false
	}
}

func writeReport(w http.ResponseWriter, log *slog.Logger, report []*Report, unavailable bool, unavailableCode int) {
	if unavailable {
		w.WriteHeader(unavailableCode)
	}

	data, _ := json.Marshal(report, jsontext.AllowInvalidUTF8(true))
	if _, err := w.Write(data); err != nil { //nolint:gosec // JSON-encoded status report
		log.Error("failed to write response", "error", err)
	}
}
