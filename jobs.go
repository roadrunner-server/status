package status

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"sync/atomic"
)

type Jobs struct {
	statusJobsRegistry    JobsChecker
	unavailableStatusCode int
	log                   *slog.Logger
	shutdownInitiated     *atomic.Pointer[bool]
}

func NewJobsHandler(jc JobsChecker, shutdownInitiated *atomic.Pointer[bool], log *slog.Logger, usc int) *Jobs {
	return &Jobs{
		statusJobsRegistry:    jc,
		unavailableStatusCode: usc,
		log:                   log,
		shutdownInitiated:     shutdownInitiated,
	}
}

func (jb *Jobs) ServeHTTP(w http.ResponseWriter, _ *http.Request) {
	if jb.shutdownInitiated != nil && *jb.shutdownInitiated.Load() {
		http.Error(w, "service is shutting down", http.StatusServiceUnavailable)
		return
	}

	if jb.statusJobsRegistry == nil {
		http.Error(w, "jobs plugin not found", jb.unavailableStatusCode)
		return
	}

	jobStates, err := jb.statusJobsRegistry.JobsState(context.Background())
	if err != nil {
		jb.log.Error("jobs state", "error", err)
		http.Error(w, "jobs plugin not found", jb.unavailableStatusCode)
		return
	}

	report := make([]*JobsReport, 0, len(jobStates))

	// write info about underlying drivers
	for i := range jobStates {
		report = append(report, &JobsReport{
			Pipeline:     jobStates[i].Pipeline,
			Priority:     jobStates[i].Priority,
			Ready:        jobStates[i].Ready,
			Queue:        jobStates[i].Queue,
			Active:       jobStates[i].Active,
			Delayed:      jobStates[i].Delayed,
			Reserved:     jobStates[i].Reserved,
			Driver:       jobStates[i].Driver,
			ErrorMessage: jobStates[i].ErrorMessage,
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
