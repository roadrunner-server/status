// Package status is a RoadRunner plugin that exposes HTTP endpoints and RPC
// methods for monitoring the health, readiness, and job queue state of
// registered plugins.
//
// The plugin starts an HTTP server with three endpoints:
//
//   - /health – returns the aggregated health status of every plugin that
//     implements the [Checker] interface.
//   - /ready  – returns the readiness status of every plugin that implements
//     the [Readiness] interface.
//   - /jobs   – returns the state of job pipelines from a plugin that
//     implements the [JobsChecker] interface.
//
// During graceful shutdown the endpoints respond with 503 Service Unavailable
// (or a configurable status code) so that external load balancers can drain
// traffic before the process exits.
//
// An RPC service is also registered, providing Status and Ready methods for
// programmatic access from RoadRunner workers or CLI tools.
package status
