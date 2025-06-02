package coderd

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
	"time"

	"golang.org/x/xerrors"

	"cdr.dev/slog"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/codersdk"
)

// ProcessRoleTokenLifetimesConfig parses the raw JSON string from deployment values,
// resolves organization names to IDs, and populates the SessionLifetime.parsedRoleLifetimes map.
// This should be called during server startup after config is loaded and DB is available.
func ProcessRoleTokenLifetimesConfig(
	ctx context.Context,
	sessionsConfig *codersdk.SessionLifetime,
	db database.Store,
	logger slog.Logger,
) error {
	rawLifetimesJSON := strings.TrimSpace(sessionsConfig.RoleTokenLifetimes.Value())
	if rawLifetimesJSON == "" || rawLifetimesJSON == "{}" {
		sessionsConfig.SetParsedRoleLifetimes(make(map[string]time.Duration))
		logger.Debug(ctx, "role token lifetimes configuration is empty, no custom lifetimes loaded")
		return nil
	}

	var rawRoleLifetimes map[string]string
	if err := json.Unmarshal([]byte(rawLifetimesJSON), &rawRoleLifetimes); err != nil {
		// This basic JSON validation should ideally be caught by DeploymentValues.Validate()
		// but double-checking here before complex parsing is fine.
		return xerrors.Errorf("failed to unmarshal role_token_lifetimes JSON: %w", err)
	}

	finalParsedLifetimes := make(map[string]time.Duration, len(rawRoleLifetimes))
	logger.Info(ctx, "processing role token lifetimes configuration", "raw_entries", len(rawRoleLifetimes))

	for key, durationStr := range rawRoleLifetimes {
		// Validate key format before processing
		if err := validateRoleTokenLifetimesKey(key); err != nil {
			logger.Error(ctx, "invalid key format in role_token_lifetimes, skipping entry",
				"key", key, "error", err)
			continue
		}

		duration, err := time.ParseDuration(durationStr)
		if err != nil {
			logger.Error(ctx, "invalid duration string in role_token_lifetimes, skipping entry. "+
				"expected format: number followed by unit (e.g., '24h', '168h', '720h')",
				"key", key, "duration", durationStr, "error", err)
			continue
		}
		if duration <= 0 {
			logger.Error(ctx, "duration in role_token_lifetimes must be positive, skipping entry", "key", key, "duration", durationStr)
			continue
		}

		parts := strings.SplitN(key, "/", 2)
		internalKey := ""
		if len(parts) == 2 { // Org-specific: "OrgName/rolename"
			orgName := strings.TrimSpace(parts[0])
			roleNamePart := strings.TrimSpace(parts[1])

			org, err := db.GetOrganizationByName(ctx, database.GetOrganizationByNameParams{Name: orgName, Deleted: false})
			if err != nil {
				// Handle missing organization (sql.ErrNoRows) as a non-critical issue.
				// This allows the server to start even if configured organization names don't exist yet.
				if xerrors.Is(err, sql.ErrNoRows) {
					logger.Warn(ctx, "organization not found for role_token_lifetimes entry, skipping", "org_name", orgName, "key", key)
					continue
				}
				// Handle unexpected database errors as critical issues that prevent server startup.
				// These might indicate connectivity problems or other serious database issues.
				logger.Error(ctx, "database error resolving organization name for role_token_lifetimes, skipping entry", "org_name", orgName, "key", key, "error", err)
				return xerrors.Errorf("database error resolving organization %q for role_token_lifetimes: %w", orgName, err)
			}
			internalKey = roleNamePart + ":" + org.ID.String()
			logger.Debug(ctx, "mapped org role for token lifetime", "config_key", key, "internal_key", internalKey, "org_name", orgName, "org_id", org.ID.String())
		} else { // Site-wide: "rolename"
			internalKey = strings.TrimSpace(key)
			if internalKey == "" {
				logger.Error(ctx, "empty site-wide role name in role_token_lifetimes, skipping", "key", key)
				continue
			}
			logger.Debug(ctx, "mapped site role for token lifetime", "internal_key", internalKey)
		}

		if _, exists := finalParsedLifetimes[internalKey]; exists {
			logger.Warn(ctx, "duplicate internal key generated for role_token_lifetimes, previous value will be overwritten", "original_key", key, "internal_key", internalKey)
		}
		finalParsedLifetimes[internalKey] = duration
	}
	sessionsConfig.SetParsedRoleLifetimes(finalParsedLifetimes)
	logger.Info(ctx, "finished processing role token lifetimes configuration", "parsed_valid_entries", len(finalParsedLifetimes))
	return nil
}

// validateRoleTokenLifetimesKey checks if the key is in a valid format.
func validateRoleTokenLifetimesKey(key string) error {
	if key == "" {
		return xerrors.New("role key cannot be empty")
	}

	slashCount := strings.Count(key, "/")
	if slashCount > 1 {
		return xerrors.Errorf("invalid format: too many '/' separators (found %d, expected 0 or 1)", slashCount)
	}

	if strings.HasPrefix(key, "/") || strings.HasSuffix(key, "/") {
		return xerrors.Errorf("invalid format: key cannot start or end with '/'")
	}

	if slashCount == 1 {
		parts := strings.Split(key, "/")
		if parts[0] == "" || parts[1] == "" {
			return xerrors.New("invalid format: organization name and role name cannot be empty")
		}
	}

	return nil
}
