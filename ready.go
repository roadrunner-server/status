package status

import (
	"fmt"
	"html"
	"net/http"
	"sync/atomic"

	"go.uber.org/zap"
)

// readinessHandler return 200OK if all plugins are ready to serve
// if one of the plugins returns status from the 5xx range, the status for all queries will be 503

type Ready struct {
	log                   *zap.Logger
	unavailableStatusCode int
	statusRegistry        map[string]Readiness
	shutdownInitiated     *atomic.Pointer[bool]
}

func NewReadyHandler(sr map[string]Readiness, shutdownInitiated *atomic.Pointer[bool], log *zap.Logger, usc int) *Ready {
	return &Ready{
		log:                   log,
		statusRegistry:        sr,
		unavailableStatusCode: usc,
		shutdownInitiated:     shutdownInitiated,
	}
}

func (rd *Ready) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if rd.shutdownInitiated != nil && *rd.shutdownInitiated.Load() {
		http.Error(w, "service is shutting down", http.StatusServiceUnavailable)
		return
	}

	if r == nil || r.URL == nil || r.URL.Query() == nil {
		http.Error(
			w,
			"No plugins provided in query. Query should be in form of: ready?plugin=plugin1&plugin=plugin2",
			http.StatusBadRequest,
		)
		return
	}

	pl := r.URL.Query()[pluginsQuery]

	if len(pl) == 0 {
		http.Error(
			w,
			"No plugins provided in query. Query should be in form of: ready?plugin=plugin1&plugin=plugin2",
			http.StatusBadRequest,
		)
		return
	}

	// iterate over all provided plugins
	for i := 0; i < len(pl); i++ {
		switch {
		// check workers for the plugin
		case rd.statusRegistry[pl[i]] != nil:
			st, errS := rd.statusRegistry[pl[i]].Ready()
			if errS != nil {
				http.Error(w, errS.Error(), rd.unavailableStatusCode)
				return
			}

			if st == nil {
				// nil can be only if the service is unavailable
				w.WriteHeader(rd.unavailableStatusCode)
				_, _ = w.Write([]byte(fmt.Sprintf(template, html.EscapeString(pl[i]), rd.unavailableStatusCode)))
				return
			}

			if st.Code >= 500 {
				// if there are 500 or 503 status codes return immediately
				w.WriteHeader(rd.unavailableStatusCode)
				_, _ = w.Write([]byte(fmt.Sprintf(template, html.EscapeString(pl[i]), rd.unavailableStatusCode)))
				return
			} else if st.Code >= 100 && st.Code <= 400 {
				_, _ = w.Write([]byte(fmt.Sprintf(template, html.EscapeString(pl[i]), st.Code)))
				continue
			}

			_, _ = w.Write([]byte(fmt.Sprintf("plugin: %s not found", html.EscapeString(pl[i]))))
			// check job drivers statuses
			// map is plugin -> states
		default:
			_, _ = w.Write([]byte(fmt.Sprintf("plugin: %s not found", html.EscapeString(pl[i]))))
		}
	}
}
