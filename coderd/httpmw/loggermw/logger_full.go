//go:build !slim

package loggermw

import (
	"context"
	"sync"
	"time"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/rbac"
)

type RequestLogger interface {
	WithFields(fields ...slog.Field)
	WriteLog(ctx context.Context, status int)
	WithAuthContext(actor rbac.Subject)
}

type RbacSlogRequestLogger struct {
	SlogRequestLogger
	// Protects actors map for concurrent writes.
	mu     sync.RWMutex
	actors map[rbac.SubjectType]rbac.Subject
}

var _ RequestLogger = &RbacSlogRequestLogger{}

func NewRequestLogger(log slog.Logger, message string, start time.Time) RequestLogger {
	rlogger := &RbacSlogRequestLogger{
		SlogRequestLogger: SlogRequestLogger{
			log:     log,
			written: false,
			message: message,
			start:   start,
		},
		actors: make(map[rbac.SubjectType]rbac.Subject),
	}
	rlogger.addFields = rlogger.addAuthContextFields
	return rlogger
}

func (c *RbacSlogRequestLogger) WithAuthContext(actor rbac.Subject) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.actors[actor.Type] = actor
}

var actorLogOrder = []rbac.SubjectType{
	rbac.SubjectTypeAutostart,
	rbac.SubjectTypeCryptoKeyReader,
	rbac.SubjectTypeCryptoKeyRotator,
	rbac.SubjectTypeDBPurge,
	rbac.SubjectTypeJobReaper,
	rbac.SubjectTypeNotifier,
	rbac.SubjectTypePrebuildsOrchestrator,
	rbac.SubjectTypeSubAgentAPI,
	rbac.SubjectTypeProvisionerd,
	rbac.SubjectTypeResourceMonitor,
	rbac.SubjectTypeSystemReadProvisionerDaemons,
	rbac.SubjectTypeSystemRestricted,
}

func (c *RbacSlogRequestLogger) addAuthContextFields() {
	c.mu.RLock()
	defer c.mu.RUnlock()

	usr, ok := c.actors[rbac.SubjectTypeUser]
	if ok {
		c.log = c.log.With(
			slog.F("requestor_id", usr.ID),
			slog.F("requestor_name", usr.FriendlyName),
			slog.F("requestor_email", usr.Email),
		)
	} else {
		// If there is no user, we log the requestor name for the first
		// actor in a defined order.
		for _, v := range actorLogOrder {
			subj, ok := c.actors[v]
			if !ok {
				continue
			}
			c.log = c.log.With(
				slog.F("requestor_name", subj.FriendlyName),
			)
			break
		}
	}
}
