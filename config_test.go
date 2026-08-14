package status

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestConfigInitDefaults(t *testing.T) {
	for _, tt := range []struct {
		name         string
		cfg          Config
		wantCode     int
		wantTimeoutS int
	}{
		{
			name:         "zero value",
			cfg:          Config{},
			wantCode:     http.StatusServiceUnavailable,
			wantTimeoutS: 60,
		},
		{
			name:         "negative check timeout",
			cfg:          Config{CheckTimeout: -1},
			wantCode:     http.StatusServiceUnavailable,
			wantTimeoutS: 60,
		},
		{
			name:         "configured values are kept",
			cfg:          Config{CheckTimeout: 5, UnavailableStatusCode: http.StatusInternalServerError},
			wantCode:     http.StatusInternalServerError,
			wantTimeoutS: 5,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			cfg := tt.cfg
			cfg.InitDefaults()

			assert.Equal(t, tt.wantCode, cfg.UnavailableStatusCode)
			assert.Equal(t, tt.wantTimeoutS, cfg.CheckTimeout)
		})
	}
}
