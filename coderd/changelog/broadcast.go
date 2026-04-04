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

	hasEntry, err := changelogStore.Has(majorMinor)
	if err != nil {
		return xerrors.Errorf("check changelog entry for version %q: %w", majorMinor, err)
	}
	if !hasEntry {
		logger.Debug(ctx, "no changelog entry for version", slog.F("version", majorMinor))
		return nil
	}

	isAlreadyNotified := func(lastNotified string) bool {
		if lastNotified == "" {
			return false
		}

		currentVersion := canonicalSemverVersion(majorMinor)
		lastNotifiedVersion := canonicalSemverVersion(lastNotified)
		if currentVersion != "" && lastNotifiedVersion != "" {
			return semver.Compare(currentVersion, lastNotifiedVersion) <= 0
		}

		return lastNotified == majorMinor
	}

	lockID := int64(database.LockIDChangelogBroadcast)
	var lastNotified string
	err = func() error {
		conn, err := sqlDB.Conn(ctx)
		if err != nil {
			return xerrors.Errorf("acquire db conn: %w", err)
		}
		defer conn.Close()

		if _, err := conn.ExecContext(ctx, "SELECT pg_advisory_lock($1)", lockID); err != nil {
			return xerrors.Errorf("acquire advisory lock: %w", err)
		}
		defer func() {
			_, _ = conn.ExecContext(ctx, "SELECT pg_advisory_unlock($1)", lockID)
		}()

		err = conn.QueryRowContext(ctx, "SELECT value FROM site_configs WHERE key = $1", changelogLastNotifiedSiteConfigKey).
			Scan(&lastNotified)
		if err != nil && !xerrors.Is(err, sql.ErrNoRows) {
			return xerrors.Errorf("query last notified version: %w", err)
		}

		return nil
	}()
	if err != nil {
		return err
	}
	if isAlreadyNotified(lastNotified) {
		logger.Debug(ctx,
			"changelog already notified for this version or newer",
			slog.F("version", majorMinor),
			slog.F("last_notified_version", lastNotified),
		)
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

	alreadyNotifiedUsers, err := changelogNotifiedUsersByVersion(ctx, sqlDB, majorMinor)
	if err != nil {
		return xerrors.Errorf("list users already notified for version %s: %w", majorMinor, err)
	}

	const pageSize = 100
	var afterID uuid.UUID
	usersNotified := 0
	enqueueFailures := 0

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
			if _, ok := alreadyNotifiedUsers[user.ID]; ok {
				continue
			}

			msgIDs, err := enqueuer.Enqueue(
				//nolint:gocritic // Enqueueing notifications requires notifier permissions.
				dbauthz.AsNotifier(ctx),
				user.ID,
				notifications.TemplateChangelog,
				labels,
				"changelog",
			)
			if err != nil {
				enqueueFailures++
				logger.Warn(ctx, "failed to enqueue changelog notification",
					slog.F("user_id", user.ID),
					slog.Error(err),
				)
				continue
			}
			if len(msgIDs) > 0 {
				usersNotified++
				alreadyNotifiedUsers[user.ID] = struct{}{}
			}
		}

		afterID = users[len(users)-1].ID
		if len(users) < pageSize {
			break
		}
	}

	if enqueueFailures > 0 {
		logger.Warn(ctx, "changelog notifications had per-user enqueue failures",
			slog.F("version", majorMinor),
			slog.F("enqueue_failures", enqueueFailures),
			slog.F("users_notified", usersNotified),
		)
		return nil
	}

	err = func() error {
		conn, err := sqlDB.Conn(ctx)
		if err != nil {
			return xerrors.Errorf("acquire db conn: %w", err)
		}
		defer conn.Close()

		if _, err := conn.ExecContext(ctx, "SELECT pg_advisory_lock($1)", lockID); err != nil {
			return xerrors.Errorf("acquire advisory lock: %w", err)
		}
		defer func() {
			_, _ = conn.ExecContext(ctx, "SELECT pg_advisory_unlock($1)", lockID)
		}()

		latestLastNotified := ""
		err = conn.QueryRowContext(ctx, "SELECT value FROM site_configs WHERE key = $1", changelogLastNotifiedSiteConfigKey).
			Scan(&latestLastNotified)
		if err != nil && !xerrors.Is(err, sql.ErrNoRows) {
			return xerrors.Errorf("query last notified version: %w", err)
		}
		if isAlreadyNotified(latestLastNotified) {
			logger.Debug(ctx,
				"changelog already notified while broadcast was in progress",
				slog.F("version", majorMinor),
				slog.F("last_notified_version", latestLastNotified),
			)
			return nil
		}

		_, err = conn.ExecContext(ctx,
			"INSERT INTO site_configs (key, value) VALUES ($1, $2) ON CONFLICT (key) DO UPDATE SET value = $2 WHERE site_configs.key = $1",
			changelogLastNotifiedSiteConfigKey,
			majorMinor,
		)
		if err != nil {
			return xerrors.Errorf("upsert last notified version: %w", err)
		}

		return nil
	}()
	if err != nil {
		return err
	}

	logger.Info(ctx, "changelog notifications sent",
		slog.F("version", majorMinor),
		slog.F("users_notified", usersNotified),
	)
	return nil
}

func canonicalSemverVersion(version string) string {
	trimmed := strings.TrimSpace(strings.TrimPrefix(version, "v"))
	if trimmed == "" {
		return ""
	}

	parts := strings.Split(trimmed, ".")
	switch len(parts) {
	case 1:
		trimmed += ".0.0"
	case 2:
		trimmed += ".0"
	}

	canonical := semver.Canonical("v" + trimmed)
	if canonical == "" {
		return ""
	}
	return canonical
}

func changelogNotifiedUsersByVersion(ctx context.Context, sqlDB *sql.DB, version string) (map[uuid.UUID]struct{}, error) {
	rows, err := sqlDB.QueryContext(ctx, `
		SELECT DISTINCT user_id
		FROM notification_messages
		WHERE notification_template_id = $1
			AND payload -> 'labels' ->> 'version' = $2
	`, notifications.TemplateChangelog, version)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	users := make(map[uuid.UUID]struct{})
	for rows.Next() {
		var userID uuid.UUID
		if err := rows.Scan(&userID); err != nil {
			return nil, err
		}
		users[userID] = struct{}{}
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return users, nil
}
