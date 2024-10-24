package status

import "net/http"

// Config is the configuration reference for the Status plugin
type Config struct {
	// Address of the http server
	Address string
	// Time to wait for a health check response.
	HealthCheckTimeout int `mapstructure:"health_check_timeout"`
	// Status code returned in case of fail, 503 by default
	UnavailableStatusCode int `mapstructure:"unavailable_status_code"`
}

// InitDefaults configuration options
func (c *Config) InitDefaults() {
	if c.UnavailableStatusCode == 0 {
		c.UnavailableStatusCode = http.StatusServiceUnavailable
	}
	if c.HealthCheckTimeout <= 0 {
		c.HealthCheckTimeout = 60
	}
}
