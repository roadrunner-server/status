package status

import (
	"context"
	"fmt"
	"net/http"

	"go.uber.org/zap"
)

const (
	jobsTemplate string = "plugin: %s: pipeline: %s | priority: %d | ready: %t | queue: %s | active: %d | delayed: %d | reserved: %d | driver: %s | error: %s \n"
)

type Jobs struct {
	statusJobsRegistry    JobsChecker
	unavailableStatusCode int
	log                   *zap.Logger
}

func NewJobsHandler(jc JobsChecker, log *zap.Logger, usc int) *Jobs {
	return &Jobs{
		statusJobsRegistry:    jc,
		unavailableStatusCode: usc,
		log:                   log,
	}
}

func (jb *Jobs) ServeHTTP(w http.ResponseWriter, _ *http.Request) {
	if jb.statusJobsRegistry == nil {
		http.Error(w, "jobs plugin not found", jb.unavailableStatusCode)
	}

	jobStates, err := jb.statusJobsRegistry.JobsState(context.Background())
	if err != nil {
		jb.log.Error("jobs state", zap.Error(err))
		return
	}

	// write info about underlying drivers
	for i := 0; i < len(jobStates); i++ {
		_, _ = w.Write([]byte(fmt.Sprintf(jobsTemplate,
			"jobs", // only JOBS plugin
			jobStates[i].Pipeline,
			jobStates[i].Priority,
			jobStates[i].Ready,
			jobStates[i].Queue,
			jobStates[i].Active,
			jobStates[i].Delayed,
			jobStates[i].Reserved,
			jobStates[i].Driver,
			jobStates[i].ErrorMessage,
		)))
	}
}
