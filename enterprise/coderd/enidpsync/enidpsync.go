package enidpsync

import (
	"cdr.dev/slog"

	"github.com/coder/coder/v2/coderd/entitlements"
	"github.com/coder/coder/v2/coderd/idpsync"
)

type EnterpriseIDPSync struct {
	entitlements *entitlements.Set
	agpl         *idpsync.AGPLIDPSync
}

func NewSync(logger slog.Logger, entitlements *entitlements.Set) *EnterpriseIDPSync {
	return &EnterpriseIDPSync{
		entitlements: entitlements,
		agpl:         idpsync.NewSync(logger),
	}
}
