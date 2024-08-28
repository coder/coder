package enidpsync

import (
	"cdr.dev/slog"

	"github.com/coder/coder/v2/coderd/entitlements"
	"github.com/coder/coder/v2/coderd/idpsync"
)

type EnterpriseIDPSync struct {
	entitlements *entitlements.Set
	*idpsync.AGPLIDPSync
}

func NewSync(logger slog.Logger, set *entitlements.Set, settings idpsync.SyncSettings) *EnterpriseIDPSync {
	return &EnterpriseIDPSync{
		entitlements: set,
		AGPLIDPSync:  idpsync.NewAGPLSync(logger.With(slog.F("enterprise_capable", "true")), settings),
	}
}
