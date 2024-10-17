package notifications

import (
	"context"
	"database/sql"
	"errors"

	"golang.org/x/xerrors"
)

func (n *notifier) fetchAppName(ctx context.Context) (string, error) {
	appName, err := n.store.GetApplicationName(ctx)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return notificationsDefaultAppName, nil
		}
		return "", xerrors.Errorf("get organization: %w", err)
	}
	return appName, nil
}

func (n *notifier) fetchLogoURL(ctx context.Context) (string, error) {
	logoURL, err := n.store.GetLogoURL(ctx)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return notificationsDefaultLogoURL, nil
		}
		return "", xerrors.Errorf("get logo URL: %w", err)
	}
	return logoURL, nil
}
