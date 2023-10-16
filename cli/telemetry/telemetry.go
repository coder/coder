package telemetry

import "time"

type Option struct {
	Name        string `json:"name"`
	ValueSource string `json:"value_source"`
}

type Invocation struct {
	Command string   `json:"command"`
	Options []Option `json:"options"`
	// InvokedAt is provided for deduplication purposes.
	InvokedAt time.Time `json:"invoked_at"`
}
