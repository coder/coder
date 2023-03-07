package database

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"
	"golang.org/x/xerrors"

	"github.com/coder/coder/coderd/database/sqlxqueries"
	"github.com/coder/coder/coderd/rbac"
	"github.com/coder/coder/coderd/rbac/regosql"
)

const (
	authorizedQueryPlaceholder = "-- @authorize_filter"
)

// customQuerier encompasses all non-generated queries.
// It provides a flexible way to write queries for cases
// where sqlc proves inadequate.
type customQuerier interface {
	templateQuerier
	workspaceQuerier
	userQuerier
}

type templateQuerier interface {
	GetAuthorizedTemplates(ctx context.Context, arg GetTemplatesWithFilterParams, prepared rbac.PreparedAuthorized) ([]Template, error)
	GetTemplateGroupRoles(ctx context.Context, id uuid.UUID) ([]TemplateGroup, error)
	GetTemplateUserRoles(ctx context.Context, id uuid.UUID) ([]TemplateUser, error)
}

func (q *sqlQuerier) GetAuthorizedTemplates(ctx context.Context, arg GetTemplatesWithFilterParams, prepared rbac.PreparedAuthorized) ([]Template, error) {
	authorizedFilter, err := prepared.CompileToSQL(ctx, regosql.ConvertConfig{
		VariableConverter: regosql.TemplateConverter(),
	})
	if err != nil {
		return nil, xerrors.Errorf("compile authorized filter: %w", err)
	}

	filtered, err := insertAuthorizedFilter(getTemplatesWithFilter, fmt.Sprintf(" AND %s", authorizedFilter))
	if err != nil {
		return nil, xerrors.Errorf("insert authorized filter: %w", err)
	}

	// The name comment is for metric tracking
	query := fmt.Sprintf("-- name: GetAuthorizedTemplates :many\n%s", filtered)
	rows, err := q.db.QueryContext(ctx, query,
		arg.Deleted,
		arg.OrganizationID,
		arg.ExactName,
		pq.Array(arg.IDs),
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []Template
	for rows.Next() {
		var i Template
		if err := rows.Scan(
			&i.ID,
			&i.CreatedAt,
			&i.UpdatedAt,
			&i.OrganizationID,
			&i.Deleted,
			&i.Name,
			&i.Provisioner,
			&i.ActiveVersionID,
			&i.Description,
			&i.DefaultTTL,
			&i.CreatedBy,
			&i.Icon,
			&i.UserACL,
			&i.GroupACL,
			&i.DisplayName,
			&i.AllowUserCancelWorkspaceJobs,
		); err != nil {
			return nil, err
		}
		items = append(items, i)
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

type TemplateUser struct {
	User
	Actions Actions `db:"actions"`
}

func (q *sqlQuerier) GetTemplateUserRoles(ctx context.Context, id uuid.UUID) ([]TemplateUser, error) {
	const query = `
	SELECT
		perms.value as actions, users.*
	FROM
		users
	JOIN
		(
			SELECT
				*
			FROM
				jsonb_each_text(
					(
						SELECT
							templates.user_acl
						FROM
							templates
						WHERE
							id = $1
					)
				)
		) AS perms
	ON
		users.id::text = perms.key
	WHERE
		users.deleted = false
	AND
		users.status = 'active';
	`

	var tus []TemplateUser
	err := q.db.SelectContext(ctx, &tus, query, id.String())
	if err != nil {
		return nil, xerrors.Errorf("select user actions: %w", err)
	}

	return tus, nil
}

type TemplateGroup struct {
	Group
	Actions Actions `db:"actions"`
}

func (q *sqlQuerier) GetTemplateGroupRoles(ctx context.Context, id uuid.UUID) ([]TemplateGroup, error) {
	const query = `
	SELECT
		perms.value as actions, groups.*
	FROM
		groups
	JOIN
		(
			SELECT
				*
			FROM
				jsonb_each_text(
					(
						SELECT
							templates.group_acl
						FROM
							templates
						WHERE
							id = $1
					)
				)
		) AS perms
	ON
		groups.id::text = perms.key;
	`

	var tgs []TemplateGroup
	err := q.db.SelectContext(ctx, &tgs, query, id.String())
	if err != nil {
		return nil, xerrors.Errorf("select group roles: %w", err)
	}

	return tgs, nil
}

type workspaceQuerier interface {
	GetAuthorizedWorkspaces(ctx context.Context, arg GetWorkspacesParams, prepared rbac.PreparedAuthorized) ([]GetWorkspacesRow, error)
	GetWorkspaceBuildByID(ctx context.Context, id uuid.UUID) (WorkspaceBuild, error)
	GetWorkspaceBuildByJobID(ctx context.Context, jobID uuid.UUID) (WorkspaceBuild, error)
	GetWorkspaceBuildsCreatedAfter(ctx context.Context, after time.Time) ([]WorkspaceBuild, error)
	GetWorkspaceBuildByWorkspaceIDAndBuildNumber(ctx context.Context, arg GetWorkspaceBuildByWorkspaceIDAndBuildNumberParams) (WorkspaceBuild, error)
	GetWorkspaceBuildsByWorkspaceID(ctx context.Context, arg GetWorkspaceBuildsByWorkspaceIDParams) ([]WorkspaceBuild, error)
	GetLatestWorkspaceBuildsByWorkspaceIDs(ctx context.Context, ids []uuid.UUID) ([]WorkspaceBuild, error)
	GetLatestWorkspaceBuilds(ctx context.Context) ([]WorkspaceBuild, error)
	GetLatestWorkspaceBuildByWorkspaceID(ctx context.Context, workspacedID uuid.UUID) (WorkspaceBuild, error)
}

type WorkspaceBuild struct {
	WorkspaceBuildThin
	OrganizationID   uuid.UUID `db:"organization_id" json:"organization_id"`
	WorkspaceOwnerID uuid.UUID `db:"workspace_owner_id" json:"workspace_owner_id"`
}

type getWorkspaceBuildParams struct {
	BuildID      uuid.UUID `db:"build_id"`
	JobID        uuid.UUID `db:"job_id"`
	CreatedAfter time.Time `db:"created_after"`
	WorkspaceID  uuid.UUID `db:"workspace_id"`
	BuildNumber  int32     `db:"build_number"`
	LimitOpt     int32     `db:"limit_opt"`
	Latest       bool      `db:"-"`
	TestLimit    int       `db:"-"`
}

func (q *sqlQuerier) getWorkspaceBuild(ctx context.Context, arg getWorkspaceBuildParams) (WorkspaceBuild, error) {
	arg.LimitOpt = 1
	return sqlxqueries.GetContext[WorkspaceBuild](ctx, q.sdb, "GetWorkspaceBuild", arg)
}

func (q *sqlQuerier) selectWorkspaceBuild(ctx context.Context, arg getWorkspaceBuildParams) ([]WorkspaceBuild, error) {
	arg.LimitOpt = -1
	return sqlxqueries.SelectContext[WorkspaceBuild](ctx, q.sdb, "GetWorkspaceBuild", arg)
}

func (q *sqlQuerier) GetWorkspaceBuildByID(ctx context.Context, id uuid.UUID) (WorkspaceBuild, error) {
	return q.getWorkspaceBuild(ctx, getWorkspaceBuildParams{BuildID: id})
}

func (q *sqlQuerier) GetWorkspaceBuildByJobID(ctx context.Context, jobID uuid.UUID) (WorkspaceBuild, error) {
	return q.getWorkspaceBuild(ctx, getWorkspaceBuildParams{JobID: jobID})
}

func (q *sqlQuerier) GetWorkspaceBuildsCreatedAfter(ctx context.Context, after time.Time) ([]WorkspaceBuild, error) {
	return q.selectWorkspaceBuild(ctx, getWorkspaceBuildParams{CreatedAfter: after})
}

type GetWorkspaceBuildByWorkspaceIDAndBuildNumberParams struct {
	BuildNumber int32     `db:"build_number"`
	WorkspaceID uuid.UUID `db:"workspace_id""`
}

func (q *sqlQuerier) GetWorkspaceBuildByWorkspaceIDAndBuildNumber(ctx context.Context, arg GetWorkspaceBuildByWorkspaceIDAndBuildNumberParams) (WorkspaceBuild, error) {
	return q.getWorkspaceBuild(ctx, getWorkspaceBuildParams{
		BuildNumber: arg.BuildNumber,
		WorkspaceID: arg.WorkspaceID,
	})
}

type GetWorkspaceBuildsByWorkspaceIDParams struct {
	WorkspaceID uuid.UUID `db:"workspace_id" json:"workspace_id"`
	Since       time.Time `db:"since" json:"since"`
	AfterID     uuid.UUID `db:"after_id" json:"after_id"`
	OffsetOpt   int32     `db:"offset_opt" json:"offset_opt"`
	LimitOpt    int32     `db:"limit_opt" json:"limit_opt"`
}

func (q *sqlQuerier) GetWorkspaceBuildsByWorkspaceID(ctx context.Context, arg GetWorkspaceBuildsByWorkspaceIDParams) ([]WorkspaceBuild, error) {
	return sqlxqueries.SelectContext[WorkspaceBuild](ctx, q.sdb, "GetWorkspaceBuildsByWorkspaceID", arg)
}

func (q *sqlQuerier) GetLatestWorkspaceBuildByWorkspaceID(ctx context.Context, workspacedID uuid.UUID) (WorkspaceBuild, error) {
	return q.getWorkspaceBuild(ctx, getWorkspaceBuildParams{WorkspaceID: workspacedID, Latest: true})
}

func (q *sqlQuerier) GetLatestWorkspaceBuildsByWorkspaceIDs(ctx context.Context, ids []uuid.UUID) ([]WorkspaceBuild, error) {
	return sqlxqueries.SelectContext[WorkspaceBuild](ctx, q.sdb, "GetLatestWorkspaceBuildsByWorkspaceIDs", ids)
}

func (q *sqlQuerier) GetLatestWorkspaceBuilds(ctx context.Context) ([]WorkspaceBuild, error) {
	return sqlxqueries.SelectContext[WorkspaceBuild](ctx, q.sdb, "GetLatestWorkspaceBuilds", nil)
}

// GetAuthorizedWorkspaces returns all workspaces that the user is authorized to access.
// This code is copied from `GetWorkspaces` and adds the authorized filter WHERE
// clause.
func (q *sqlQuerier) GetAuthorizedWorkspaces(ctx context.Context, arg GetWorkspacesParams, prepared rbac.PreparedAuthorized) ([]GetWorkspacesRow, error) {
	authorizedFilter, err := prepared.CompileToSQL(ctx, rbac.ConfigWithoutACL())
	if err != nil {
		return nil, xerrors.Errorf("compile authorized filter: %w", err)
	}

	// In order to properly use ORDER BY, OFFSET, and LIMIT, we need to inject the
	// authorizedFilter between the end of the where clause and those statements.
	filtered, err := insertAuthorizedFilter(getWorkspaces, fmt.Sprintf(" AND %s", authorizedFilter))
	if err != nil {
		return nil, xerrors.Errorf("insert authorized filter: %w", err)
	}

	// The name comment is for metric tracking
	query := fmt.Sprintf("-- name: GetAuthorizedWorkspaces :many\n%s", filtered)
	rows, err := q.db.QueryContext(ctx, query,
		arg.Deleted,
		arg.Status,
		arg.OwnerID,
		arg.OwnerUsername,
		arg.TemplateName,
		pq.Array(arg.TemplateIds),
		arg.Name,
		arg.HasAgent,
		arg.AgentInactiveDisconnectTimeoutSeconds,
		arg.Offset,
		arg.Limit,
	)
	if err != nil {
		return nil, xerrors.Errorf("get authorized workspaces: %w", err)
	}
	defer rows.Close()
	var items []GetWorkspacesRow
	for rows.Next() {
		var i GetWorkspacesRow
		if err := rows.Scan(
			&i.ID,
			&i.CreatedAt,
			&i.UpdatedAt,
			&i.OwnerID,
			&i.OrganizationID,
			&i.TemplateID,
			&i.Deleted,
			&i.Name,
			&i.AutostartSchedule,
			&i.Ttl,
			&i.LastUsedAt,
			&i.Count,
		); err != nil {
			return nil, err
		}
		items = append(items, i)
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

type userQuerier interface {
	GetAuthorizedUserCount(ctx context.Context, arg GetFilteredUserCountParams, prepared rbac.PreparedAuthorized) (int64, error)
}

func (q *sqlQuerier) GetAuthorizedUserCount(ctx context.Context, arg GetFilteredUserCountParams, prepared rbac.PreparedAuthorized) (int64, error) {
	authorizedFilter, err := prepared.CompileToSQL(ctx, rbac.ConfigWithoutACL())
	if err != nil {
		return -1, xerrors.Errorf("compile authorized filter: %w", err)
	}

	filtered, err := insertAuthorizedFilter(getFilteredUserCount, fmt.Sprintf(" AND %s", authorizedFilter))
	if err != nil {
		return -1, xerrors.Errorf("insert authorized filter: %w", err)
	}

	query := fmt.Sprintf("-- name: GetAuthorizedUserCount :one\n%s", filtered)
	row := q.db.QueryRowContext(ctx, query,
		arg.Search,
		pq.Array(arg.Status),
		pq.Array(arg.RbacRole),
	)
	var count int64
	err = row.Scan(&count)
	return count, err
}

func insertAuthorizedFilter(query string, replaceWith string) (string, error) {
	if !strings.Contains(query, authorizedQueryPlaceholder) {
		return "", xerrors.Errorf("query does not contain authorized replace string, this is not an authorized query")
	}
	filtered := strings.Replace(query, authorizedQueryPlaceholder, replaceWith, 1)
	return filtered, nil
}
