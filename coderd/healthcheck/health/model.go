package health

import (
	"fmt"
	"strings"

	"github.com/coder/coder/v2/buildinfo"
	"github.com/coder/coder/v2/coderd/util/ptr"
)

const (
	SeverityOK      Severity = "ok"
	SeverityWarning Severity = "warning"
	SeverityError   Severity = "error"

	// CodeUnknown is a catch-all health code when something unexpected goes wrong (for example, a panic).
	CodeUnknown Code = "EUNKNOWN"

	CodeProxyUpdate Code = "EWP01"
	CodeProxyFetch  Code = "EWP02"
	// CodeProxyVersionMismatch is no longer used as it's no longer a critical
	// error.
	// CodeProxyVersionMismatch Code = "EWP03"
	CodeProxyUnhealthy Code = "EWP04"

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
	CodeSTUNNoNodes                = `ESTUN01`
	CodeSTUNMapVaryDest            = `ESTUN02`

	CodeProvisionerDaemonsNoProvisionerDaemons     Code = `EPD01`
	CodeProvisionerDaemonVersionMismatch           Code = `EPD02`
	CodeProvisionerDaemonAPIMajorVersionDeprecated Code = `EPD03`

	CodeInterfaceSmallMTU = `EIF01`
)

// Default docs URL
var (
	docsURLDefault = "https://coder.com/docs/v2"
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

// URL returns a link to the admin/healthcheck docs page for the given Message.
// NOTE: if using a custom docs URL, specify base.
func (m Message) URL(base string) string {
	var codeAnchor string
	if m.Code == "" {
		codeAnchor = strings.ToLower(string(CodeUnknown))
	} else {
		codeAnchor = strings.ToLower(string(m.Code))
	}

	if base == "" {
		base = docsURLDefault
		versionPath := buildinfo.Version()
		if buildinfo.IsDev() {
			// for development versions, just use latest
			versionPath = "latest"
		}
		return fmt.Sprintf("%s/%s/admin/healthcheck#%s", base, versionPath, codeAnchor)
	}

	// We don't assume that custom docs URLs are versioned.
	return fmt.Sprintf("%s/admin/healthcheck#%s", base, codeAnchor)
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
