package dbauthz

import (
	"context"
	"encoding/json"
	"strings"

	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/rbac/policy"
	"github.com/coder/coder/v2/codersdk"
)

// The minute aggregation queries take the app-to-family registry as jsonb and
// fall back to the unknown family for unregistered apps, so a bad registry
// still succeeds while silently misattributing usage. dbauthz wraps every
// production store, including the rollup's transaction stores, so validating
// here fails the call loudly instead. These methods override the generated
// ones in dbauthz.go; scripts/dbgen preserves methods defined outside that
// file.

// validateSessionCountAppFamilies checks that the registry is a non-empty
// jsonb object of normalized app names to normalized, non-unknown family
// names. New families are valid without any SQL change.
func validateSessionCountAppFamilies(appFamilies json.RawMessage) error {
	var families map[string]codersdk.AppFamilyName
	if len(appFamilies) > 0 {
		if err := json.Unmarshal(appFamilies, &families); err != nil {
			return xerrors.Errorf("invalid app family registry: %w", err)
		}
	}
	if len(families) == 0 {
		return xerrors.New("app family registry is empty")
	}

	for appName, family := range families {
		if appName == "" {
			return xerrors.New("empty app name")
		}
		// Stored app names are normalized, so an unnormalized key matches no
		// session and that app's activity falls back to the unknown family.
		if normalized := codersdk.NormalizeAppName(appName); normalized != appName {
			return xerrors.Errorf("app name %q not normalized, want %q", appName, normalized)
		}
		// A blank family is not an attribution, so its apps would report under
		// no usable name at all.
		if strings.TrimSpace(string(family)) == "" {
			return xerrors.Errorf("no family for app %q", appName)
		}
		if normalized := codersdk.NormalizeAppName(string(family)); normalized != string(family) {
			return xerrors.Errorf("family %q for app %q not normalized, want %q", family, appName, normalized)
		}
		if family == codersdk.AppFamilyUnknown {
			return xerrors.Errorf("app %q maps to unknown family", appName)
		}
	}
	return nil
}

func (q *querier) GetTemplateInsightsByTemplate(ctx context.Context, arg database.GetTemplateInsightsByTemplateParams) ([]database.GetTemplateInsightsByTemplateRow, error) {
	// Only used by prometheus metrics collector. No need to check update template perms.
	if err := q.authorizeContext(ctx, policy.ActionViewInsights, rbac.ResourceTemplate); err != nil {
		return nil, err
	}
	if err := validateSessionCountAppFamilies(arg.AppFamilies); err != nil {
		return nil, err
	}
	return q.db.GetTemplateInsightsByTemplate(ctx, arg)
}

func (q *querier) UpsertTemplateUsageStats(ctx context.Context, appFamilies json.RawMessage) error {
	if err := q.authorizeContext(ctx, policy.ActionUpdate, rbac.ResourceSystem); err != nil {
		return err
	}
	if err := validateSessionCountAppFamilies(appFamilies); err != nil {
		return err
	}
	return q.db.UpsertTemplateUsageStats(ctx, appFamilies)
}
