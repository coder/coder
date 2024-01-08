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

	CodeProvisionerDaemonsNoProvisionerDaemons     Code = `EPD01`
	CodeProvisionerDaemonVersionMismatch           Code = `EPD02`
	CodeProvisionerDaemonAPIMajorVersionDeprecated Code = `EPD03`
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

// @typescript-generate Message
type Message struct {
	Code    Code   `json:"code"`
	Message string `json:"message"`
}

func (m Message) String() string {
	var sb strings.Builder
	_, _ = sb.WriteString(string(m.Code))
	_, _ = sb.WriteRune(':')
	_, _ = sb.WriteRune(' ')
	_, _ = sb.WriteString(m.Message)
	return sb.String()
}

// Code is a stable identifier used to link to documentation.
// @typescript-generate Code
type Code string

// Messagef is a convenience function for returning a health.Message
func Messagef(code Code, msg string, args ...any) Message {
	return Message{
		Code:    code,
		Message: fmt.Sprintf(msg, args...),
	}
}

// Errorf is a convenience function for returning a stringly-typed version of a Message.
func Errorf(code Code, msg string, args ...any) *string {
	return ptr.Ref(Messagef(code, msg, args...).String())
}
