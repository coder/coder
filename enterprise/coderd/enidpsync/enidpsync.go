package enidpsync

import (
	"cdr.dev/slog"
	"github.com/coder/coder/v2/coderd/entitlements"
	"github.com/coder/coder/v2/coderd/idpsync"
	"github.com/coder/coder/v2/coderd/runtimeconfig"
)

var _ idpsync.IDPSync = &EnterpriseIDPSync{}

// EnterpriseIDPSync enabled syncing user information from an external IDP.
// The sync is an enterprise feature, so this struct wraps the AGPL implementation
// and extends it with enterprise capabilities. These capabilities can entirely
// be changed in the Parsing, and leaving the "syncing" part (which holds the
// more complex logic) to the shared AGPL implementation.
type EnterpriseIDPSync struct {
	entitlements *entitlements.Set
	*idpsync.AGPLIDPSync
}

func NewSync(logger slog.Logger, manager *runtimeconfig.Manager, set *entitlements.Set, settings idpsync.DeploymentSyncSettings) *EnterpriseIDPSync {
	return &EnterpriseIDPSync{
		entitlements: set,
		AGPLIDPSync:  idpsync.NewAGPLSync(logger.With(slog.F("enterprise_capable", "true")), manager, settings),
	}
}
