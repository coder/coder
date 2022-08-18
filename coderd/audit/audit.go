package audit

import (
	"context"

	"github.com/coder/coder/coderd/database"
)

type Auditor interface {
	Export(ctx context.Context, alog database.AuditLog) error
	diff(old, new any) Map
}
