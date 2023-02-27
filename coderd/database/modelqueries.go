package database

import (
	"context"
	"database/sql"

	"github.com/coder/coder/coderd/database/sqlxqueries"

	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/lib/pq"
	"golang.org/x/xerrors"

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
	GetAuthorizedWorkspaces(ctx context.Context, arg GetWorkspacesParams, prepared rbac.PreparedAuthorized) ([]WorkspaceWithData, error)
	GetWorkspaces(ctx context.Context, arg GetWorkspacesParams) ([]WorkspaceWithData, error)
	GetWorkspaceByID(ctx context.Context, id uuid.UUID) (WorkspaceWithData, error)
	GetWorkspaceByOwnerIDAndName(ctx context.Context, arg GetWorkspaceByOwnerIDAndNameParams) (WorkspaceWithData, error)
}

// WorkspaceWithData includes related information to the workspace.
type WorkspaceWithData struct {
	Workspace `db:""`

	// User related fields.
	OwnerUserName                  string `db:"owner_username"`
	LatestBuildInitiatorUsername   string `db:"latest_build_initiator_username"`
	LatestBuildTemplateVersionName string `db:"latest_build_template_version_name"`

	// These template fields are included in the response for a workspace.
	// This means if you can read a workspace, you can also read these limited
	// template fields as they are metadata of the workspace.
	TemplateName                         string    `db:"template_name"`
	TemplateIcon                         string    `db:"template_icon"`
	TemplateDisplayName                  string    `db:"template_display_name"`
	TemplateAllowUserCancelWorkspaceJobs bool      `db:"template_allow_user_cancel"`
	TemplateActiveVersionID              uuid.UUID `db:"template_active_version_id"`

	LatestBuild    WorkspaceBuild `db:"latest_build"`
	LatestBuildJob ProvisionerJob `db:"latest_build_job"`

	// Count is the total number of workspaces applicable to the query.
	// This is used for pagination as the total number of returned workspaces
	// could be less than this number.
	Count int64 `db:"count" json:"count"`
}

type GetWorkspacesParams struct {
	Deleted                               bool        `db:"deleted" json:"deleted"`
	Status                                string      `db:"status" json:"status"`
	OwnerID                               uuid.UUID   `db:"owner_id" json:"owner_id"`
	OwnerUsername                         string      `db:"owner_username" json:"owner_username"`
	TemplateName                          string      `db:"template_name" json:"template_name"`
	TemplateIds                           []uuid.UUID `db:"template_ids" json:"template_ids"`
	WorkspaceIds                          []uuid.UUID `db:"workspace_ids" json:"workspace_ids"`
	Name                                  string      `db:"name" json:"name"`
	ExactName                             string      `db:"exact_name" json:"exact_name"`
	HasAgent                              string      `db:"has_agent" json:"has_agent"`
	AgentInactiveDisconnectTimeoutSeconds int64       `db:"agent_inactive_disconnect_timeout_seconds" json:"agent_inactive_disconnect_timeout_seconds"`
	Offset                                int32       `db:"offset_" json:"offset_"`
	Limit                                 int32       `db:"limit_" json:"limit_"`
}

func (q *sqlQuerier) GetWorkspaces(ctx context.Context, arg GetWorkspacesParams) ([]WorkspaceWithData, error) {
	return q.GetAuthorizedWorkspaces(ctx, arg, NoopAuthorizer{})
}

type GetWorkspaceByOwnerIDAndNameParams struct {
	OwnerID uuid.UUID `db:"owner_id" json:"owner_id"`
	Deleted bool      `db:"deleted" json:"deleted"`
	Name    string    `db:"name" json:"name"`
}

func (q *sqlQuerier) GetWorkspaceByOwnerIDAndName(ctx context.Context, arg GetWorkspaceByOwnerIDAndNameParams) (WorkspaceWithData, error) {
	workspaces, err := q.GetAuthorizedWorkspaces(ctx, GetWorkspacesParams{
		Deleted:   arg.Deleted,
		ExactName: arg.Name,
		OwnerID:   arg.OwnerID,
		Limit:     1,
	}, NoopAuthorizer{})
	if err != nil {
		return WorkspaceWithData{}, err
	}
	if len(workspaces) == 0 {
		return WorkspaceWithData{}, sql.ErrNoRows
	}
	return workspaces[0], nil
}

func (q *sqlQuerier) GetWorkspaceByID(ctx context.Context, id uuid.UUID) (WorkspaceWithData, error) {
	workspaces, err := q.GetAuthorizedWorkspaces(ctx, GetWorkspacesParams{
		WorkspaceIds: []uuid.UUID{id},
		Limit:        1,
	}, NoopAuthorizer{})
	if err != nil {
		return WorkspaceWithData{}, err
	}
	if len(workspaces) == 0 {
		return WorkspaceWithData{}, sql.ErrNoRows
	}
	return workspaces[0], nil
}

// GetAuthorizedWorkspaces returns all workspaces that the user is authorized to access.
// This code is copied from `GetWorkspaces` and adds the authorized filter WHERE
// clause.
func (q *sqlQuerier) GetAuthorizedWorkspaces(ctx context.Context, arg GetWorkspacesParams, prepared rbac.PreparedAuthorized) ([]WorkspaceWithData, error) {
	authorizedFilter, err := prepared.CompileToSQL(ctx, rbac.ConfigWithoutACL("workspaces."))
	if err != nil {
		return nil, xerrors.Errorf("compile authorized filter: %w", err)
	}

	getAuthorizedWorkspacesQuery, err := sqlxqueries.GetAuthorizedWorkspaces()
	if err != nil {
		return nil, xerrors.Errorf("get query: %w", err)
	}

	// In order to properly use ORDER BY, OFFSET, and LIMIT, we need to inject the
	// authorizedFilter between the end of the where clause and those statements.
	filtered, err := insertAuthorizedFilter(getAuthorizedWorkspacesQuery, fmt.Sprintf(" AND %s", authorizedFilter))
	if err != nil {
		return nil, xerrors.Errorf("insert authorized filter: %w", err)
	}

	query, args, err := bindNamed(filtered, arg)
	if err != nil {
		return nil, xerrors.Errorf("bind named: %w", err)
	}

	var items []WorkspaceWithData
	// SelectContext maps the results of the query to the items slice by struct
	// db tags.
	err = q.sdb.SelectContext(ctx, &items, query, args...)
	if err != nil {
		return nil, xerrors.Errorf("get authorized workspaces: %w", err)
	}

	return items, nil
}

type userQuerier interface {
	GetAuthorizedUserCount(ctx context.Context, arg GetFilteredUserCountParams, prepared rbac.PreparedAuthorized) (int64, error)
}

func (q *sqlQuerier) GetAuthorizedUserCount(ctx context.Context, arg GetFilteredUserCountParams, prepared rbac.PreparedAuthorized) (int64, error) {
	authorizedFilter, err := prepared.CompileToSQL(ctx, rbac.ConfigWithoutACL(""))
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

// TODO: This can be removed once dbauthz becomes the default.
type NoopAuthorizer struct{}

func (NoopAuthorizer) Authorize(ctx context.Context, object rbac.Object) error {
	return nil
}
func (NoopAuthorizer) CompileToSQL(ctx context.Context, cfg regosql.ConvertConfig) (string, error) {
	return "", nil
}
