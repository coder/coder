package db2sdk

import (
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/codersdk"
)

// AIGatewayPolicy converts a policy row and its versions to the SDK form.
func AIGatewayPolicy(row database.AIGatewayPolicy, versions []database.AIGatewayPolicyVersion) codersdk.AIGatewayPolicy {
	out := codersdk.AIGatewayPolicy{
		ID:          row.ID,
		Name:        row.Name,
		DisplayName: row.DisplayName.String,
		Kind:        codersdk.AIGatewayPolicyKind(row.Kind),
		CreatedAt:   row.CreatedAt,
		UpdatedAt:   row.UpdatedAt,
	}
	if row.ActiveVersionID.Valid {
		id := row.ActiveVersionID.UUID
		out.ActiveVersionID = &id
	}
	for _, v := range versions {
		out.Versions = append(out.Versions, AIGatewayPolicyVersion(v))
	}
	return out
}

// AIGatewayPolicyVersion converts a policy version row to the SDK form.
func AIGatewayPolicyVersion(row database.AIGatewayPolicyVersion) codersdk.AIGatewayPolicyVersion {
	out := codersdk.AIGatewayPolicyVersion{
		ID:                  row.ID,
		PolicyID:            row.PolicyID,
		VersionNumber:       row.VersionNumber,
		Rego:                row.Rego,
		InputSchemaVersion:  row.InputSchemaVersion,
		OutputSchemaVersion: row.OutputSchemaVersion,
		Description:         row.Description.String,
		CreatedAt:           row.CreatedAt,
	}
	if row.CreatedBy.Valid {
		id := row.CreatedBy.UUID
		out.CreatedBy = &id
	}
	return out
}

// AIGatewayPipeline converts a pipeline row and its active version members to
// the SDK form. activeMembers may be nil when the pipeline has no active
// version.
func AIGatewayPipeline(row database.AIGatewayPipeline, activeVersion *database.AIGatewayPipelineVersion, activeMembers []database.AIGatewayPipelineVersionPolicy) codersdk.AIGatewayPipeline {
	out := codersdk.AIGatewayPipeline{
		ID:         row.ID,
		ProviderID: row.ProviderID,
		Enabled:    row.Enabled,
		CreatedAt:  row.CreatedAt,
		UpdatedAt:  row.UpdatedAt,
	}
	if row.ActiveVersionID.Valid {
		id := row.ActiveVersionID.UUID
		out.ActiveVersionID = &id
	}
	if activeVersion != nil {
		v := AIGatewayPipelineVersion(*activeVersion, activeMembers)
		out.ActiveVersion = &v
	}
	return out
}

// AIGatewayPipelineVersion converts a pipeline version row and its members.
func AIGatewayPipelineVersion(row database.AIGatewayPipelineVersion, members []database.AIGatewayPipelineVersionPolicy) codersdk.AIGatewayPipelineVersion {
	out := codersdk.AIGatewayPipelineVersion{
		ID:            row.ID,
		PipelineID:    row.PipelineID,
		VersionNumber: row.VersionNumber,
		CreatedAt:     row.CreatedAt,
		Policies:      make([]codersdk.AIGatewayPipelinePolicy, 0, len(members)),
	}
	for _, m := range members {
		out.Policies = append(out.Policies, codersdk.AIGatewayPipelinePolicy{
			PolicyVersionID: m.PolicyVersionID,
			Hook:            codersdk.AIGatewayHook(m.Hook),
			Kind:            codersdk.AIGatewayPolicyKind(m.Kind),
			FailMode:        codersdk.AIGatewayFailMode(m.FailMode),
			Enabled:         m.Enabled,
		})
	}
	return out
}
