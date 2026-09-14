package dbtestutil

import (
	"context"
	"database/sql"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"

	"github.com/coder/coder/v2/coderd/database"
)

// GetWorkspaceBuildOrchestrationByParentBuildID reads a workspace
// build orchestration row directly from the database for tests.
//
// It scans into the struct by column name so new columns are picked up
// automatically without updating this helper.
func GetWorkspaceBuildOrchestrationByParentBuildID(
	ctx context.Context,
	sqlDB *sql.DB,
	parentBuildID uuid.UUID,
) (database.WorkspaceBuildOrchestration, error) {
	db := sqlx.NewDb(sqlDB, "postgres")
	var orchestration database.WorkspaceBuildOrchestration
	err := db.GetContext(
		ctx,
		&orchestration,
		`SELECT * FROM workspace_build_orchestrations WHERE parent_build_id = $1`,
		parentBuildID,
	)
	return orchestration, err
}
