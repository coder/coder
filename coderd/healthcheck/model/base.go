package model

const (
	SeverityOK      Severity = "ok"
	SeverityWarning Severity = "warning"
	SeverityError   Severity = "error"
)

// @typescript-generate Severity
type Severity string

type HealthSummary struct {
	Healthy  bool     `json:"healthy"`
	Severity Severity `json:"severity" enums:"ok,warning,error"`
	Warnings []string `json:"warnings"`
}
