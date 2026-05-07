package dbtestutil

import (
	"context"
	"database/sql"

	"github.com/google/uuid"

	"github.com/coder/coder/v2/coderd/database"
)

const getWorkspaceBuildOrchestrationByParentBuildIDQuery = `
SELECT
	id,
	created_at,
	updated_at,
	parent_build_id,
	child_build_id,
	child_transition,
	child_template_version_id,
	child_template_version_preset_id,
	child_rich_parameter_values,
	child_log_level,
	child_reason,
	attempt_count,
	next_retry_after,
	status,
	error
FROM
	workspace_build_orchestrations
WHERE
	parent_build_id = $1
`

// GetWorkspaceBuildOrchestrationByParentBuildID reads a workspace
// build orchestration row directly from the database for tests.
func GetWorkspaceBuildOrchestrationByParentBuildID(
	ctx context.Context,
	sqlDB *sql.DB,
	parentBuildID uuid.UUID,
) (database.WorkspaceBuildOrchestration, error) {
	var orchestration database.WorkspaceBuildOrchestration
	err := sqlDB.QueryRowContext(
		ctx,
		getWorkspaceBuildOrchestrationByParentBuildIDQuery,
		parentBuildID,
	).Scan(
		&orchestration.ID,
		&orchestration.CreatedAt,
		&orchestration.UpdatedAt,
		&orchestration.ParentBuildID,
		&orchestration.ChildBuildID,
		&orchestration.ChildTransition,
		&orchestration.ChildTemplateVersionID,
		&orchestration.ChildTemplateVersionPresetID,
		&orchestration.ChildRichParameterValues,
		&orchestration.ChildLogLevel,
		&orchestration.ChildReason,
		&orchestration.AttemptCount,
		&orchestration.NextRetryAfter,
		&orchestration.Status,
		&orchestration.Error,
	)
	return orchestration, err
}
