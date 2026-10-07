package audit

import (
	"context"
	"strconv"

	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
)

// WorkspaceBuildSecret is the metadata of a secret linked to a workspace
// build. Secret values are never audited.
type WorkspaceBuildSecret struct {
	Name     string                         `json:"name"`
	EnvName  string                         `json:"env_name"`
	FilePath string                         `json:"file_path"`
	Source   database.WorkspaceSecretSource `json:"source"`
}

// WorkspaceBuildFields returns the additional fields for a workspace build
// audit entry, including the full set of secrets linked to the build.
// Removed secrets have no row, so they are absent from the set. If the
// secrets cannot be listed, the other fields are still returned with the
// error.
func WorkspaceBuildFields(ctx context.Context, db database.Store, workspace database.Workspace, build database.WorkspaceBuild) (AdditionalFields, error) {
	fields := AdditionalFields{
		WorkspaceName: workspace.Name,
		BuildNumber:   strconv.FormatInt(int64(build.BuildNumber), 10),
		BuildReason:   build.Reason,
		WorkspaceID:   workspace.ID,
	}

	//nolint:gocritic // Audit entries list secret metadata for every build initiator; values are never read.
	rows, err := db.GetWorkspaceSecretsHistory(dbauthz.AsWorkspaceSecretManager(ctx), database.GetWorkspaceSecretsHistoryParams{
		WorkspaceID:      workspace.ID,
		WorkspaceBuildID: build.ID,
	})
	if err != nil {
		return fields, xerrors.Errorf("list workspace build secrets: %w", err)
	}
	for _, row := range rows {
		fields.WorkspaceSecrets = append(fields.WorkspaceSecrets, WorkspaceBuildSecret{
			Name:     row.Name,
			EnvName:  row.EnvName,
			FilePath: row.FilePath,
			Source:   row.Source,
		})
	}
	return fields, nil
}
