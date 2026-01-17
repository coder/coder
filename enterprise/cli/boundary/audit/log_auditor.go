//nolint:revive,gocritic,errname,unconvert
package audit

import "log/slog"

// LogAuditor implements proxy.Auditor by logging to slog
type LogAuditor struct {
	logger *slog.Logger
}

// NewLogAuditor creates a new LogAuditor
func NewLogAuditor(logger *slog.Logger) *LogAuditor {
	return &LogAuditor{
		logger: logger,
	}
}

// AuditRequest logs the request using structured logging
func (a *LogAuditor) AuditRequest(req Request) {
	if req.Allowed {
		a.logger.Info("ALLOW",
			"method", req.Method,
			"url", req.URL,
			"host", req.Host,
			"rule", req.Rule)
	} else {
		a.logger.Warn("DENY",
			"method", req.Method,
			"url", req.URL,
			"host", req.Host,
		)
	}
}
