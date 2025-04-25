package status

import (
	"encoding/json"
	"net/http"
	"sync/atomic"

	"go.uber.org/zap"
)

type Health struct {
	log                   *zap.Logger
	unavailableStatusCode int
	statusRegistry        map[string]Checker
	shutdownInitiated     *atomic.Pointer[bool]
}

func NewHealthHandler(sr map[string]Checker, shutdownInitiated *atomic.Pointer[bool], log *zap.Logger, usc int) *Health {
	return &Health{
		statusRegistry:        sr,
		unavailableStatusCode: usc,
		log:                   log,
		shutdownInitiated:     shutdownInitiated,
	}
}

func (rd *Health) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if rd.shutdownInitiated != nil && *rd.shutdownInitiated.Load() {
		http.Error(w, "service is shutting down", http.StatusOK)
		return
	}

	// report will be used either for all plugins or for the Plugins in the query
	report := make([]*Report, 0, 2)

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

				rd.log.Info("plugin is nil or not initialized", zap.String("plugin", k))
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
			rd.log.Error("failed to marshal response", zap.Error(err))
			return
		}
		// write the response
		_, err = w.Write(data)
		if err != nil {
			rd.log.Error("failed to write response", zap.Error(err))
		}

		return
	}

	// iterate over all provided Plugins
	for i := range plg {
		if svc, ok := rd.statusRegistry[plg[i]]; ok {
			if svc == nil {
				continue
			}

			st, err := rd.statusRegistry[plg[i]].Status()
			if err != nil {
				report = append(report, &Report{
					PluginName:   plg[i],
					ErrorMessage: err.Error(),
					StatusCode:   http.StatusInternalServerError,
				})

				continue
			}

			if st == nil {
				report = append(report, &Report{
					PluginName:   plg[i],
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
					PluginName:   plg[i],
					ErrorMessage: "internal server error, see logs",
					StatusCode:   rd.unavailableStatusCode,
				})
			case st.Code >= 100 && st.Code <= 400:
				report = append(report, &Report{
					PluginName: plg[i],
					StatusCode: st.Code,
				})
			default:
				report = append(report, &Report{
					PluginName:   plg[i],
					ErrorMessage: "unexpected status code",
					StatusCode:   st.Code,
				})
			}
		} else {
			rd.log.Info("plugin does not support health checks", zap.String("plugin", plg[i]))
		}
	}

	data, err := json.Marshal(report)
	if err != nil {
		rd.log.Error("failed to marshal response", zap.Error(err))
	}

	// write the response
	_, err = w.Write(data)
	if err != nil {
		rd.log.Error("failed to write response", zap.Error(err))
	}
}
