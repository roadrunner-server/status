package status

import (
	"encoding/json"
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

	// report will be used either for all plugins or for the Plugins in the query
	report := make([]*Report, 0, len(rd.statusRegistry))

	plg := r.URL.Query()[pluginsQuery]
	// if no Plugins provided, check them all
	if len(plg) == 0 {
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

			st, err := pl.Ready()
			if err != nil {
				report = append(report, &Report{
					PluginName:   k,
					ErrorMessage: err.Error(),
					StatusCode:   http.StatusInternalServerError,
				})

				continue
			}

			if st == nil {
				report = append(report, &Report{
					PluginName:   k,
					ErrorMessage: "plugin is not available",
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
			rd.log.Info("plugin does not support readiness checks", "plugin", name)
			continue
		}

		if svc == nil {
			continue
		}

		st, err := svc.Ready()
		if err != nil {
			w.WriteHeader(rd.unavailableStatusCode)
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
