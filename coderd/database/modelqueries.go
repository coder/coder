package database

import (
	"context"
	"database/sql/driver"
	"embed"
	"fmt"
	"strings"

	_ "embed"

	"github.com/google/uuid"
	"github.com/lib/pq"
	"golang.org/x/xerrors"

	"github.com/coder/coder/coderd/rbac"
	"github.com/coder/coder/coderd/rbac/regosql"
)

//go:embed sqlxqueries/*.sql
var sqlxQueries embed.FS

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
}

// WorkspaceWithData includes information returned by the api for a workspace.
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

type UUIDs []uuid.UUID

func (ids UUIDs) Value() (driver.Value, error) {
	v := pq.Array(ids)
	return v.Value()
}

func (ids *UUIDs) Scan(src interface{}) error {
	v := pq.Array(ids)
	return v.Scan(src)
}

type GetWorkspacesParams struct {
	Deleted                               bool      `db:"deleted" json:"deleted"`
	Status                                string    `db:"status" json:"status"`
	OwnerID                               uuid.UUID `db:"owner_id" json:"owner_id"`
	OwnerUsername                         string    `db:"owner_username" json:"owner_username"`
	TemplateName                          string    `db:"template_name" json:"template_name"`
	TemplateIds                           UUIDs     `db:"template_ids" json:"template_ids"`
	WorkspaceIds                          UUIDs     `db:"workspace_ids" json:"workspace_ids"`
	Name                                  string    `db:"name" json:"name"`
	HasAgent                              string    `db:"has_agent" json:"has_agent"`
	AgentInactiveDisconnectTimeoutSeconds int64     `db:"agent_inactive_disconnect_timeout_seconds" json:"agent_inactive_disconnect_timeout_seconds"`
	Offset                                int32     `db:"offset_" json:"offset_"`
	Limit                                 int32     `db:"limit_" json:"limit_"`
}

// GetAuthorizedWorkspaces returns all workspaces that the user is authorized to access.
// This code is copied from `GetWorkspaces` and adds the authorized filter WHERE
// clause.
func (q *sqlQuerier) GetAuthorizedWorkspaces(ctx context.Context, arg GetWorkspacesParams, prepared rbac.PreparedAuthorized) ([]WorkspaceWithData, error) {
	authorizedFilter, err := prepared.CompileToSQL(ctx, rbac.ConfigWithoutACL("workspaces."))
	if err != nil {
		return nil, xerrors.Errorf("compile authorized filter: %w", err)
	}

	getQuery, err := sqlxQueries.ReadFile("sqlxqueries/getworkspaces.sql")
	if err != nil {
		panic("developer error")
	}

	// In order to properly use ORDER BY, OFFSET, and LIMIT, we need to inject the
	// authorizedFilter between the end of the where clause and those statements.
	filtered, err := insertAuthorizedFilter(string(getQuery), fmt.Sprintf(" AND %s", authorizedFilter))
	if err != nil {
		return nil, xerrors.Errorf("insert authorized filter: %w", err)
	}

	// SQLx expects :arg-named arguments, but we use @arg-named arguments. So
	// switch them. Also any ':' in comments breaks this...
	filtered = strings.ReplaceAll(filtered, "@", ":")
	query, args, err := q.sdb.BindNamed(filtered, arg)
	if err != nil {
		return nil, xerrors.Errorf("bind named: %w", err)
	}
	// So SQLx treats '::' as escaping a ":". So we need to unescape it??
	query = strings.ReplaceAll(query, ":", "::")

	// The name comment is for metric tracking
	// Must add after sqlx or else it breaks 'BindNamed'
	query = fmt.Sprintf("-- name: GetAuthorizedWorkspaces :many\n%s", query)
	var items []WorkspaceWithData
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
