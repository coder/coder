package health

const (
	SeverityOK      Severity = "ok"
	SeverityWarning Severity = "warning"
	SeverityError   Severity = "error"
)

// @typescript-generate Severity
type Severity string

var severityRank = map[Severity]int{
	SeverityOK:      0,
	SeverityWarning: 1,
	SeverityError:   2,
}

func (s Severity) Value() int {
	return severityRank[s]
}
