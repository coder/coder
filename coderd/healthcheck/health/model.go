package health

import (
	"fmt"
	"strings"
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

// Code is a stable identifier used to link to documentation.
// @typescript-generate Code
type Code string

// Messagef is a convenience function for formatting a healthcheck error message.
func Messagef(code Code, msg string, args ...any) string {
	var sb strings.Builder
	_, _ = sb.WriteString(string(code))
	_, _ = sb.WriteRune(':')
	_, _ = sb.WriteRune(' ')
	_, _ = sb.WriteString(fmt.Sprintf(msg, args...))
	return sb.String()
}
