package status

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"sync/atomic"
)

type Jobs struct {
	statusJobsRegistry    JobsChecker
	unavailableStatusCode int
	log                   *slog.Logger
	shutdownInitiated     *atomic.Bool
}

func NewJobsHandler(jc JobsChecker, shutdownInitiated *atomic.Bool, log *slog.Logger, usc int) *Jobs {
	return &Jobs{
		statusJobsRegistry:    jc,
		unavailableStatusCode: usc,
		log:                   log,
		shutdownInitiated:     shutdownInitiated,
	}
}

func (jb *Jobs) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if jb.shutdownInitiated != nil && jb.shutdownInitiated.Load() {
		http.Error(w, "service is shutting down", http.StatusServiceUnavailable)
		return
	}

	if jb.statusJobsRegistry == nil {
		http.Error(w, "jobs plugin not found", jb.unavailableStatusCode)
		return
	}

	jobStates, err := jb.statusJobsRegistry.JobsState(r.Context())
	if err != nil {
		jb.log.Error("jobs state", "error", err)
		http.Error(w, "jobs plugin not found", jb.unavailableStatusCode)
		return
	}

	report := make([]*JobsReport, 0, len(jobStates))

	// write info about underlying drivers
	for _, js := range jobStates {
		report = append(report, &JobsReport{
			Pipeline:     js.Pipeline,
			Priority:     js.Priority,
			Ready:        js.Ready,
			Queue:        js.Queue,
			Active:       js.Active,
			Delayed:      js.Delayed,
			Reserved:     js.Reserved,
			Driver:       js.Driver,
			ErrorMessage: js.ErrorMessage,
		})
	}

	data, err := json.Marshal(report)
	if err != nil {
		jb.log.Error("failed to marshal jobs state report", "error", err)
		return
	}

	_, err = w.Write(data)
	if err != nil {
		jb.log.Error("failed to write jobs state report", "error", err)
	}
}
