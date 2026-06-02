package status

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"sync/atomic"
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

	// report will be used either for all plugins or for the Plugins in the query
	report := make([]*Report, 0, len(rd.statusRegistry))

	plg := r.URL.Query()[pluginsQuery]
	// if no Plugins provided, check them all
	if len(plg) == 0 {
		rd.log.Debug("no plugins provided, checking all plugins")

		for k, pl := range rd.statusRegistry {
			if pl == nil {
				report = append(report, &Report{
					PluginName:   k,
					ErrorMessage: "plugin is nil or not initialized",
					StatusCode:   http.StatusNotFound,
				})

				rd.log.Info("plugin is nil or not initialized", "plugin", k)
				continue
			}

			st, err := pl.Status()
			if err != nil {
				w.WriteHeader(rd.unavailableStatusCode)
				report = append(report, &Report{
					PluginName:   k,
					ErrorMessage: err.Error(),
					StatusCode:   rd.unavailableStatusCode,
				})
				continue
			}

			if st == nil {
				report = append(report, &Report{
					PluginName:   k,
					ErrorMessage: "plugin is not available, returned nil",
					StatusCode:   rd.unavailableStatusCode,
				})
				continue
			}

			switch {
			case st.Code >= 500:
				w.WriteHeader(rd.unavailableStatusCode)

				report = append(report, &Report{
					PluginName:   k,
					ErrorMessage: "internal server error, see logs",
					StatusCode:   rd.unavailableStatusCode,
				})
			case st.Code >= 100 && st.Code <= 400:
				report = append(report, &Report{
					PluginName: k,
					StatusCode: st.Code,
				})
			default:
				report = append(report, &Report{
					PluginName:   k,
					ErrorMessage: "unexpected status code",
					StatusCode:   st.Code,
				})
			}
		}

		data, err := json.Marshal(report)
		if err != nil {
			// TODO do we need to write this error to the ResponseWriter?
			rd.log.Error("failed to marshal response", "error", err)
			return
		}
		// write the response
		_, err = w.Write(data)
		if err != nil {
			rd.log.Error("failed to write response", "error", err)
		}

		return
	}

	// iterate over all provided Plugins
	for _, name := range plg {
		svc, ok := rd.statusRegistry[name]
		if !ok {
			rd.log.Info("plugin does not support health checks", "plugin", name)
			continue
		}

		if svc == nil {
			continue
		}

		st, err := svc.Status()
		if err != nil {
			report = append(report, &Report{
				PluginName:   name,
				ErrorMessage: err.Error(),
				StatusCode:   http.StatusInternalServerError,
			})

			continue
		}

		if st == nil {
			report = append(report, &Report{
				PluginName:   name,
				ErrorMessage: "plugin is not available",
				StatusCode:   rd.unavailableStatusCode,
			})

			continue
		}

		switch {
		case st.Code >= 500:
			// on >=500, write header, because it'll be written on Write (200)
			w.WriteHeader(rd.unavailableStatusCode)
			report = append(report, &Report{
				PluginName:   name,
				ErrorMessage: "internal server error, see logs",
				StatusCode:   rd.unavailableStatusCode,
			})
		case st.Code >= 100 && st.Code <= 400:
			report = append(report, &Report{
				PluginName: name,
				StatusCode: st.Code,
			})
		default:
			report = append(report, &Report{
				PluginName:   name,
				ErrorMessage: "unexpected status code",
				StatusCode:   st.Code,
			})
		}
	}

	data, err := json.Marshal(report)
	if err != nil {
		rd.log.Error("failed to marshal response", "error", err)
	}

	// write the response
	_, err = w.Write(data)
	if err != nil {
		rd.log.Error("failed to write response", "error", err)
	}
}
