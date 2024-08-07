package status

import (
	"fmt"
	"html"
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

	if r == nil || r.URL == nil || r.URL.Query() == nil {
		http.Error(
			w,
			"No plugins provided in query. Query should be in form of: health?plugin=plugin1&plugin=plugin2",
			http.StatusBadRequest,
		)
		return
	}

	pl := r.URL.Query()[pluginsQuery]

	if len(pl) == 0 {
		http.Error(
			w,
			"No plugins provided in query. Query should be in form of: health?plugin=plugin1&plugin=plugin2",
			http.StatusBadRequest,
		)
		return
	}

	// iterate over all provided plugins
	for i := 0; i < len(pl); i++ {
		switch {
		// check workers for the plugin
		case rd.statusRegistry[pl[i]] != nil:
			st, errS := rd.statusRegistry[pl[i]].Status()
			if errS != nil {
				http.Error(w, errS.Error(), rd.unavailableStatusCode)
				return
			}

			if st == nil {
				w.WriteHeader(rd.unavailableStatusCode)
				// nil can be only if the service unavailable
				_, _ = w.Write([]byte(fmt.Sprintf(template, html.EscapeString(pl[i]), rd.unavailableStatusCode)))
				return
			}

			switch {
			case st.Code >= 500:
				w.WriteHeader(rd.unavailableStatusCode)
				// if there is 500 or 503 status code return immediately
				_, _ = w.Write([]byte(fmt.Sprintf(template, html.EscapeString(pl[i]), rd.unavailableStatusCode)))
				return
			case st.Code >= 100 && st.Code <= 400:
				_, _ = w.Write([]byte(fmt.Sprintf(template, html.EscapeString(pl[i]), st.Code)))
				continue
			default:
				_, _ = w.Write([]byte(fmt.Sprintf("plugin: %s not found", html.EscapeString(pl[i]))))
			}
		default:
			_, _ = w.Write([]byte(fmt.Sprintf("plugin: %s not found", html.EscapeString(pl[i]))))
		}
	}
}
