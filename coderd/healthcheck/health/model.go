package health

import (
	"fmt"
	"strings"

	"github.com/coder/coder/v2/coderd/util/ptr"
)

const (
	SeverityOK      Severity = "ok"
	SeverityWarning Severity = "warning"
	SeverityError   Severity = "error"

	// CodeUnknown is a catch-all health code when something unexpected goes wrong (for example, a panic).
	CodeUnknown Code = "EUNKNOWN"

	CodeProxyUpdate          Code = "EWP01"
	CodeProxyFetch           Code = "EWP02"
	CodeProxyVersionMismatch Code = "EWP03"
	CodeProxyUnhealthy       Code = "EWP04"

	CodeDatabasePingFailed Code = "EDB01"
	CodeDatabasePingSlow   Code = "EDB02"

	CodeWebsocketDial Code = "EWS01"
	CodeWebsocketEcho Code = "EWS02"
	CodeWebsocketMsg  Code = "EWS03"

	CodeAccessURLNotSet  Code = "EACS01"
	CodeAccessURLInvalid Code = "EACS02"
	CodeAccessURLFetch   Code = "EACS03"
	CodeAccessURLNotOK   Code = "EACS04"

	CodeDERPNodeUsesWebsocket Code = `EDERP01`
	CodeDERPOneNodeUnhealthy  Code = `EDERP02`
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

// @typescript-generate Warning
type Warning struct {
	Code    Code
	Message string
}

func (w Warning) String() string {
	var sb strings.Builder
	_, _ = sb.WriteString(string(w.Code))
	_, _ = sb.WriteRune(':')
	_, _ = sb.WriteRune(' ')
	_, _ = sb.WriteString(w.Message)
	return sb.String()
}

// Code is a stable identifier used to link to documentation.
// @typescript-generate Code
type Code string

// Warnf is a convenience function for returning a health.Warning
func Warnf(code Code, msg string, args ...any) Warning {
	return Warning{
		Code:    code,
		Message: fmt.Sprintf(msg, args...),
	}
}

// Errorf is a convenience function for returning a stringly-typed version of a Warning.
func Errorf(code Code, msg string, args ...any) *string {
	return ptr.Ref(Warnf(code, msg, args...).String())
}
