package db2sdk

import (
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/codersdk"
)

// AIGatewayGuardrail converts a guardrail row and its versions to the SDK form.
func AIGatewayGuardrail(row database.AIGatewayGuardrail, versions []database.AIGatewayGuardrailVersion) codersdk.AIGatewayGuardrail {
	out := codersdk.AIGatewayGuardrail{
		ID:          row.ID,
		Name:        row.Name,
		DisplayName: row.DisplayName.String,
		AdapterType: row.AdapterType,
		Enabled:     row.Enabled,
		CreatedAt:   row.CreatedAt,
		UpdatedAt:   row.UpdatedAt,
	}
	if row.ActiveVersionID.Valid {
		id := row.ActiveVersionID.UUID
		out.ActiveVersionID = &id
	}
	for _, v := range versions {
		out.Versions = append(out.Versions, AIGatewayGuardrailVersion(v))
	}
	return out
}

// AIGatewayGuardrailVersion converts a guardrail version row to the SDK form.
// The credential is never serialized; only its presence is reported.
func AIGatewayGuardrailVersion(row database.AIGatewayGuardrailVersion) codersdk.AIGatewayGuardrailVersion {
	out := codersdk.AIGatewayGuardrailVersion{
		ID:            row.ID,
		GuardrailID:   row.GuardrailID,
		VersionNumber: row.VersionNumber,
		Config:        row.Config,
		HasCredential: row.Credential != "",
		Description:   row.Description.String,
		CreatedAt:     row.CreatedAt,
	}
	if row.CreatedBy.Valid {
		id := row.CreatedBy.UUID
		out.CreatedBy = &id
	}
	return out
}
