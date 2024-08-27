package enidpsync

import (
	"cdr.dev/slog"

	"github.com/coder/coder/v2/coderd/entitlements"
	"github.com/coder/coder/v2/coderd/idpsync"
)

func init() {
	idpsync.NewSync = func(logger slog.Logger, entitlements *entitlements.Set, settings idpsync.SyncSettings) idpsync.IDPSync {
		return NewSync(logger, entitlements, settings)
	}
}

type EnterpriseIDPSync struct {
	entitlements *entitlements.Set
	*idpsync.AGPLIDPSync
}

func NewSync(logger slog.Logger, set *entitlements.Set, settings idpsync.SyncSettings) *EnterpriseIDPSync {
	return &EnterpriseIDPSync{
		entitlements: set,
		AGPLIDPSync:  idpsync.NewAGPLSync(logger.With(slog.F("enterprise_capable", "true")), set, settings),
	}
}
