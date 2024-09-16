package status

type Report struct {
	PluginName   string `json:"plugin_name"`
	ErrorMessage string `json:"error_message"`
	StatusCode   int    `json:"status_code"`
}

type JobsReport struct {
	Pipeline     string `json:"pipeline"`
	Priority     uint64 `json:"priority"`
	Ready        bool   `json:"ready"`
	Queue        string `json:"queue"`
	Active       int64  `json:"active"`
	Delayed      int64  `json:"delayed"`
	Reserved     int64  `json:"reserved"`
	Driver       string `json:"driver"`
	ErrorMessage string `json:"error_message"`
}
