package dynamicparameters

import "github.com/google/uuid"

type Config struct {
	TemplateVersion   uuid.UUID `json:"template_version"`
	Metrics           *Metrics  `json:"-"`
	MetricLabelValues []string  `json:"metric_label_values"`
}
