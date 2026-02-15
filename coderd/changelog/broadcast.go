package changelog

import (
	"context"
	"database/sql"
	"strings"

	"github.com/google/uuid"
	"golang.org/x/mod/semver"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/buildinfo"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/notifications"
)

const changelogLastNotifiedSiteConfigKey = "changelog_last_notified_version"

// BroadcastChangelog sends a changelog notification to all active users for the
// current version, if it hasn't been sent yet.
//
// It acquires a Postgres advisory lock for HA safety.
func BroadcastChangelog(
	ctx context.Context,
	logger slog.Logger,
	sqlDB *sql.DB,
	store database.Store,
	enqueuer notifications.Enqueuer,
	changelogStore *Store,
) error {
	version := buildinfo.Version()
	if version == "" || version == "v0.0.0" {
		// No version is attached, meaning this is a dev build outside CI.
		return nil
	}

	majorMinor := strings.TrimPrefix(semver.MajorMinor(version), "v")
	if majorMinor == "" {
		return nil
	}

	if !changelogStore.Has(majorMinor) {
		logger.Debug(ctx, "no changelog entry for version", slog.F("version", majorMinor))
		return nil
	}

	conn, err := sqlDB.Conn(ctx)
	if err != nil {
		return xerrors.Errorf("acquire db conn: %w", err)
	}
	defer conn.Close()

	lockID := int64(database.LockIDChangelogBroadcast)
	_, err = conn.ExecContext(ctx, "SELECT pg_advisory_lock($1)", lockID)
	if err != nil {
		return xerrors.Errorf("acquire advisory lock: %w", err)
	}
	defer func() {
		_, _ = conn.ExecContext(ctx, "SELECT pg_advisory_unlock($1)", lockID)
	}()

	var lastNotified string
	err = conn.QueryRowContext(ctx, "SELECT value FROM site_configs WHERE key = $1", changelogLastNotifiedSiteConfigKey).
		Scan(&lastNotified)
	if err != nil && !xerrors.Is(err, sql.ErrNoRows) {
		return xerrors.Errorf("query last notified version: %w", err)
	}
	if lastNotified == majorMinor {
		logger.Debug(ctx, "changelog already notified for version", slog.F("version", majorMinor))
		return nil
	}

	entry, err := changelogStore.Get(majorMinor)
	if err != nil {
		return xerrors.Errorf("get changelog entry: %w", err)
	}

	labels := map[string]string{
		"version": majorMinor,
		"summary": entry.Summary,
	}

	const pageSize = 100
	var afterID uuid.UUID
	usersNotified := 0

	for {
		//nolint:gocritic // This needs system access to list all active users.
		users, err := store.GetUsers(dbauthz.AsSystemRestricted(ctx), database.GetUsersParams{
			AfterID:       afterID,
			Status:        []database.UserStatus{database.UserStatusActive},
			IncludeSystem: false,
			LimitOpt:      pageSize,
		})
		if err != nil {
			return xerrors.Errorf("list users: %w", err)
		}
		if len(users) == 0 {
			break
		}

		for _, user := range users {
			msgIDs, err := enqueuer.Enqueue(
				//nolint:gocritic // Enqueueing notifications requires notifier permissions.
				dbauthz.AsNotifier(ctx),
				user.ID,
				notifications.TemplateChangelog,
				labels,
				"changelog",
			)
			if err != nil {
				logger.Warn(ctx, "failed to enqueue changelog notification",
					slog.F("user_id", user.ID),
					slog.Error(err),
				)
				continue
			}
			if len(msgIDs) > 0 {
				usersNotified++
			}
		}

		afterID = users[len(users)-1].ID
		if len(users) < pageSize {
			break
		}
	}

	_, err = conn.ExecContext(ctx,
		"INSERT INTO site_configs (key, value) VALUES ($1, $2) ON CONFLICT (key) DO UPDATE SET value = $2 WHERE site_configs.key = $1",
		changelogLastNotifiedSiteConfigKey,
		majorMinor,
	)
	if err != nil {
		return xerrors.Errorf("upsert last notified version: %w", err)
	}

	logger.Info(ctx, "changelog notifications sent",
		slog.F("version", majorMinor),
		slog.F("users_notified", usersNotified),
	)
	return nil
}
