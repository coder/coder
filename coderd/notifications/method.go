package notifications

import (
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/codersdk"
)

func dispatchMethodFromCfg(cfg codersdk.NotificationsConfig) (database.NotificationMethod, error) {
	var method database.NotificationMethod
	if err := method.Scan(cfg.Method.String()); err != nil {
		return "", xerrors.Errorf("given notification method %q is invalid", cfg.Method)
	}
	return method, nil
}
