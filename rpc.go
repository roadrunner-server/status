package status

import (
	"context"
	"log/slog"

	"connectrpc.com/connect"
	statusV2 "github.com/roadrunner-server/api-go/v6/status/v2"
	"github.com/roadrunner-server/errors"
)

type rpc struct {
	srv *Plugin
	log *slog.Logger
}

// Status returns the current status of the provided plugin.
func (r *rpc) Status(_ context.Context, req *connect.Request[statusV2.StatusRequest]) (*connect.Response[statusV2.StatusResponse], error) {
	const op = errors.Op("checker_rpc_status")
	plugin := req.Msg.GetPlugin()
	r.log.Debug("Status method was invoked", "plugin", plugin)

	resp := &statusV2.StatusResponse{}
	st, err := r.srv.status(plugin)
	if err != nil {
		resp.Message = err.Error()
		return nil, connect.NewError(connect.CodeInternal, errors.E(op, err))
	}

	if st != nil {
		resp.Code = int64(st.Code)
		r.log.Debug("status code", "code", st.Code)
	}

	r.log.Debug("successfully finished the Status method")
	return connect.NewResponse(resp), nil
}

// Ready returns the readiness check of the provided plugin.
func (r *rpc) Ready(_ context.Context, req *connect.Request[statusV2.StatusRequest]) (*connect.Response[statusV2.StatusResponse], error) {
	const op = errors.Op("checker_rpc_ready")
	plugin := req.Msg.GetPlugin()
	r.log.Debug("Ready method was invoked", "plugin", plugin)

	resp := &statusV2.StatusResponse{}
	st, err := r.srv.ready(plugin)
	if err != nil {
		resp.Message = err.Error()
		return nil, connect.NewError(connect.CodeInternal, errors.E(op, err))
	}

	if st != nil {
		resp.Code = int64(st.Code)
		r.log.Debug("status code", "code", st.Code)
	}

	r.log.Debug("successfully finished the Ready method")
	return connect.NewResponse(resp), nil
}
