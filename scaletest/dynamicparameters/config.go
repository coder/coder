package dynamicparameters

import "github.com/google/uuid"

type Config struct {
	TemplateVersion   uuid.UUID `json:"template_version"`
	SessionToken      string    `json:"session_token"`
	Metrics           *Metrics  `json:"-"`
	MetricLabelValues []string  `json:"metric_label_values"`
}
