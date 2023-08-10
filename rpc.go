package status

import (
	statusv1beta1 "github.com/roadrunner-server/api/v4/build/status/v1"
	"github.com/roadrunner-server/errors"
	"go.uber.org/zap"
)

type rpc struct {
	srv *Plugin
	log *zap.Logger
}

// Status return current status of the provided plugin
func (rpc *rpc) Status(req *statusv1beta1.Request, resp *statusv1beta1.Response) error {
	const op = errors.Op("checker_rpc_status")
	rpc.log.Debug("Status method was invoked", zap.String("plugin", req.GetPlugin()))
	st, err := rpc.srv.status(req.GetPlugin())
	if err != nil {
		resp.Message = err.Error()
		return errors.E(op, err)
	}

	if st != nil {
		resp.Code = int64(st.Code)
	}

	rpc.log.Debug("status code", zap.Int("code", st.Code))
	rpc.log.Debug("successfully finished the Status method")
	return nil
}

// Ready return the readiness check of the provided plugin
func (rpc *rpc) Ready(req *statusv1beta1.Request, resp *statusv1beta1.Response) error {
	const op = errors.Op("checker_rpc_ready")
	rpc.log.Debug("Ready method was invoked", zap.String("plugin", req.GetPlugin()))
	st, err := rpc.srv.ready(req.GetPlugin())
	if err != nil {
		resp.Message = err.Error()
		return errors.E(op, err)
	}

	if st != nil {
		resp.Code = int64(st.Code)
	}

	rpc.log.Debug("status code", zap.Int("code", st.Code))
	rpc.log.Debug("successfully finished the Ready method")
	return nil
}
