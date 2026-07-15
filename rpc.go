package status

import (
	"log/slog"

	statusV2 "github.com/roadrunner-server/api-go/v6/status/v2"
	"github.com/roadrunner-server/errors"
)

type rpc struct {
	srv *Plugin
	log *slog.Logger
}

// Status returns the current status of the provided plugin.
func (r *rpc) Status(in *statusV2.StatusRequest, out *statusV2.StatusResponse) error {
	const op = errors.Op("checker_rpc_status")
	plugin := in.GetPlugin()
	r.log.Debug("Status method was invoked", "plugin", plugin)

	st, err := r.srv.status(plugin)
	if err != nil {
		return errors.E(op, err)
	}

	if st != nil {
		out.Code = int64(st.Code)
		r.log.Debug("status code", "code", st.Code)
	}

	r.log.Debug("successfully finished the Status method")
	return nil
}

// Ready returns the readiness check of the provided plugin.
func (r *rpc) Ready(in *statusV2.StatusRequest, out *statusV2.StatusResponse) error {
	const op = errors.Op("checker_rpc_ready")
	plugin := in.GetPlugin()
	r.log.Debug("Ready method was invoked", "plugin", plugin)

	st, err := r.srv.ready(plugin)
	if err != nil {
		return errors.E(op, err)
	}

	if st != nil {
		out.Code = int64(st.Code)
		r.log.Debug("status code", "code", st.Code)
	}

	r.log.Debug("successfully finished the Ready method")
	return nil
}
