package dbauthz

import (
	"context"
	"encoding/json"
	"slices"

	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/rbac/policy"
	"github.com/coder/coder/v2/codersdk"
)

// The session count read queries take the app family attribution registry as
// a single jsonb parameter. Every query hardcodes one probe per family, so a
// registry that is empty, malformed, or keyed differently from
// codersdk.AttributedAppFamilies would still run and return zero counts for
// the affected families, silently dropping sessions from deployment stats,
// insights, Prometheus, and telemetry. dbauthz wraps every production store,
// including the transaction stores used by the rollup, so validating here
// makes a wrong registry fail the call loudly instead of failing the data
// quietly. These methods override the generated ones in dbauthz.go;
// scripts/dbgen preserves methods defined outside that file.

// validateSessionCountAppFamilies checks that the registry has exactly the
// families the queries probe, each with at least one app name.
func validateSessionCountAppFamilies(appFamilies json.RawMessage) error {
	if len(appFamilies) == 0 {
		return xerrors.New("developer error: session count app families must not be empty, populate them with codersdk.SessionCountAppFamiliesJSON()")
	}

	var families map[codersdk.AppFamilyName][]string
	if err := json.Unmarshal(appFamilies, &families); err != nil {
		return xerrors.Errorf("developer error: session count app families must be a JSON object of family to app names, populate them with codersdk.SessionCountAppFamiliesJSON(): %w", err)
	}

	required := codersdk.AttributedAppFamilies()
	for _, family := range required {
		appNames, ok := families[family]
		if !ok {
			return xerrors.Errorf("developer error: session count app families is missing family %q, which the queries probe; populate them with codersdk.SessionCountAppFamiliesJSON()", family)
		}
		if len(appNames) == 0 {
			return xerrors.Errorf("developer error: session count app families has no app names for family %q, so its sessions would go uncounted", family)
		}
	}
	for family := range families {
		if !slices.Contains(required, family) {
			return xerrors.Errorf("developer error: session count app families has family %q, which no query probes; add a probe per query or drop it from codersdk.AttributedAppFamilies", family)
		}
	}
	return nil
}

func (q *querier) GetDeploymentWorkspaceAgentStats(ctx context.Context, arg database.GetDeploymentWorkspaceAgentStatsParams) (database.GetDeploymentWorkspaceAgentStatsRow, error) {
	if err := validateSessionCountAppFamilies(arg.AppFamilies); err != nil {
		return database.GetDeploymentWorkspaceAgentStatsRow{}, err
	}
	return q.db.GetDeploymentWorkspaceAgentStats(ctx, arg)
}

func (q *querier) GetDeploymentWorkspaceAgentUsageStats(ctx context.Context, arg database.GetDeploymentWorkspaceAgentUsageStatsParams) (database.GetDeploymentWorkspaceAgentUsageStatsRow, error) {
	if err := validateSessionCountAppFamilies(arg.AppFamilies); err != nil {
		return database.GetDeploymentWorkspaceAgentUsageStatsRow{}, err
	}
	return q.db.GetDeploymentWorkspaceAgentUsageStats(ctx, arg)
}

func (q *querier) GetWorkspaceAgentStats(ctx context.Context, arg database.GetWorkspaceAgentStatsParams) ([]database.GetWorkspaceAgentStatsRow, error) {
	if err := validateSessionCountAppFamilies(arg.AppFamilies); err != nil {
		return nil, err
	}
	return q.db.GetWorkspaceAgentStats(ctx, arg)
}

func (q *querier) GetWorkspaceAgentStatsAndLabels(ctx context.Context, arg database.GetWorkspaceAgentStatsAndLabelsParams) ([]database.GetWorkspaceAgentStatsAndLabelsRow, error) {
	if err := validateSessionCountAppFamilies(arg.AppFamilies); err != nil {
		return nil, err
	}
	return q.db.GetWorkspaceAgentStatsAndLabels(ctx, arg)
}

func (q *querier) GetWorkspaceAgentUsageStats(ctx context.Context, arg database.GetWorkspaceAgentUsageStatsParams) ([]database.GetWorkspaceAgentUsageStatsRow, error) {
	if err := validateSessionCountAppFamilies(arg.AppFamilies); err != nil {
		return nil, err
	}
	return q.db.GetWorkspaceAgentUsageStats(ctx, arg)
}

func (q *querier) GetWorkspaceAgentUsageStatsAndLabels(ctx context.Context, arg database.GetWorkspaceAgentUsageStatsAndLabelsParams) ([]database.GetWorkspaceAgentUsageStatsAndLabelsRow, error) {
	if err := validateSessionCountAppFamilies(arg.AppFamilies); err != nil {
		return nil, err
	}
	return q.db.GetWorkspaceAgentUsageStatsAndLabels(ctx, arg)
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
