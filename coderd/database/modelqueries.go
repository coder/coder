package database

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/lib/pq"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/rbac/regosql"
)

const (
	authorizedQueryPlaceholder = "-- @authorize_filter"
)

// ExpectOne can be used to convert a ':many:' query into a ':one'
// query. To reduce the quantity of SQL queries, a :many with a filter is used.
// These filters sometimes are expected to return just 1 row.
//
// A :many query will never return a sql.ErrNoRows, but a :one does.
// This function will correct the error for the empty set.
func ExpectOne[T any](ret []T, err error) (T, error) {
	var empty T
	if err != nil {
		return empty, err
	}

	if len(ret) == 0 {
		return empty, sql.ErrNoRows
	}

	if len(ret) > 1 {
		return empty, xerrors.Errorf("too many rows returned, expected 1")
	}

	return ret[0], nil
}

// customQuerier encompasses all non-generated queries.
// It provides a flexible way to write queries for cases
// where sqlc proves inadequate.
type customQuerier interface {
	templateQuerier
	workspaceQuerier
	userQuerier
	auditLogQuerier
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
		arg.FuzzyName,
		pq.Array(arg.IDs),
		arg.Deprecated,
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
			&i.AllowUserAutostart,
			&i.AllowUserAutostop,
			&i.FailureTTL,
			&i.TimeTilDormant,
			&i.TimeTilDormantAutoDelete,
			&i.AutostopRequirementDaysOfWeek,
			&i.AutostopRequirementWeeks,
			&i.AutostartBlockDaysOfWeek,
			&i.RequireActiveVersion,
			&i.Deprecated,
			&i.ActivityBump,
			&i.MaxPortSharingLevel,
			&i.CreatedByAvatarURL,
			&i.CreatedByUsername,
			&i.OrganizationName,
			&i.OrganizationDisplayName,
			&i.OrganizationIcon,
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
		users.status != 'suspended';
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
	GetAuthorizedWorkspacesAndAgentsByOwnerID(ctx context.Context, ownerID uuid.UUID, prepared rbac.PreparedAuthorized) ([]GetWorkspacesAndAgentsByOwnerIDRow, error)
}

// GetAuthorizedWorkspaces returns all workspaces that the user is authorized to access.
// This code is copied from `GetWorkspaces` and adds the authorized filter WHERE
// clause.
func (q *sqlQuerier) GetAuthorizedWorkspaces(ctx context.Context, arg GetWorkspacesParams, prepared rbac.PreparedAuthorized) ([]GetWorkspacesRow, error) {
	authorizedFilter, err := prepared.CompileToSQL(ctx, rbac.ConfigWorkspaces())
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
		pq.Array(arg.ParamNames),
		pq.Array(arg.ParamValues),
		arg.Deleted,
		arg.Status,
		arg.OwnerID,
		arg.OrganizationID,
		pq.Array(arg.HasParam),
		arg.OwnerUsername,
		arg.TemplateName,
		pq.Array(arg.TemplateIDs),
		pq.Array(arg.WorkspaceIds),
		arg.Name,
		arg.HasAgent,
		arg.AgentInactiveDisconnectTimeoutSeconds,
		arg.Dormant,
		arg.LastUsedBefore,
		arg.LastUsedAfter,
		arg.UsingActive,
		arg.RequesterID,
		arg.Offset,
		arg.Limit,
		arg.WithSummary,
	)
	if err != nil {
		return nil, err
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
			&i.DormantAt,
			&i.DeletingAt,
			&i.AutomaticUpdates,
			&i.Favorite,
			&i.NextStartAt,
			&i.OwnerAvatarUrl,
			&i.OwnerUsername,
			&i.OrganizationName,
			&i.OrganizationDisplayName,
			&i.OrganizationIcon,
			&i.OrganizationDescription,
			&i.TemplateName,
			&i.TemplateDisplayName,
			&i.TemplateIcon,
			&i.TemplateDescription,
			&i.TemplateVersionID,
			&i.TemplateVersionName,
			&i.LatestBuildCompletedAt,
			&i.LatestBuildCanceledAt,
			&i.LatestBuildError,
			&i.LatestBuildTransition,
			&i.LatestBuildStatus,
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

func (q *sqlQuerier) GetAuthorizedWorkspacesAndAgentsByOwnerID(ctx context.Context, ownerID uuid.UUID, prepared rbac.PreparedAuthorized) ([]GetWorkspacesAndAgentsByOwnerIDRow, error) {
	authorizedFilter, err := prepared.CompileToSQL(ctx, rbac.ConfigWorkspaces())
	if err != nil {
		return nil, xerrors.Errorf("compile authorized filter: %w", err)
	}

	// In order to properly use ORDER BY, OFFSET, and LIMIT, we need to inject the
	// authorizedFilter between the end of the where clause and those statements.
	filtered, err := insertAuthorizedFilter(getWorkspacesAndAgentsByOwnerID, fmt.Sprintf(" AND %s", authorizedFilter))
	if err != nil {
		return nil, xerrors.Errorf("insert authorized filter: %w", err)
	}

	// The name comment is for metric tracking
	query := fmt.Sprintf("-- name: GetAuthorizedWorkspacesAndAgentsByOwnerID :many\n%s", filtered)
	rows, err := q.db.QueryContext(ctx, query, ownerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []GetWorkspacesAndAgentsByOwnerIDRow
	for rows.Next() {
		var i GetWorkspacesAndAgentsByOwnerIDRow
		if err := rows.Scan(
			&i.ID,
			&i.Name,
			&i.JobStatus,
			&i.Transition,
			pq.Array(&i.Agents),
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
	GetAuthorizedUsers(ctx context.Context, arg GetUsersParams, prepared rbac.PreparedAuthorized) ([]GetUsersRow, error)
}

func (q *sqlQuerier) GetAuthorizedUsers(ctx context.Context, arg GetUsersParams, prepared rbac.PreparedAuthorized) ([]GetUsersRow, error) {
	authorizedFilter, err := prepared.CompileToSQL(ctx, regosql.ConvertConfig{
		VariableConverter: regosql.UserConverter(),
	})
	if err != nil {
		return nil, xerrors.Errorf("compile authorized filter: %w", err)
	}

	filtered, err := insertAuthorizedFilter(getUsers, fmt.Sprintf(" AND %s", authorizedFilter))
	if err != nil {
		return nil, xerrors.Errorf("insert authorized filter: %w", err)
	}

	query := fmt.Sprintf("-- name: GetAuthorizedUsers :many\n%s", filtered)
	rows, err := q.db.QueryContext(ctx, query,
		arg.AfterID,
		arg.Search,
		pq.Array(arg.Status),
		pq.Array(arg.RbacRole),
		arg.LastSeenBefore,
		arg.LastSeenAfter,
		arg.CreatedBefore,
		arg.CreatedAfter,
		arg.IncludeSystem,
		arg.GithubComUserID,
		pq.Array(arg.LoginType),
		arg.OffsetOpt,
		arg.LimitOpt,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []GetUsersRow
	for rows.Next() {
		var i GetUsersRow
		if err := rows.Scan(
			&i.ID,
			&i.Email,
			&i.Username,
			&i.HashedPassword,
			&i.CreatedAt,
			&i.UpdatedAt,
			&i.Status,
			&i.RBACRoles,
			&i.LoginType,
			&i.AvatarURL,
			&i.Deleted,
			&i.LastSeenAt,
			&i.QuietHoursSchedule,
			&i.Name,
			&i.GithubComUserID,
			&i.HashedOneTimePasscode,
			&i.OneTimePasscodeExpiresAt,
			&i.IsSystem,
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

type auditLogQuerier interface {
	GetAuthorizedAuditLogsOffset(ctx context.Context, arg GetAuditLogsOffsetParams, prepared rbac.PreparedAuthorized) ([]GetAuditLogsOffsetRow, error)
}

func (q *sqlQuerier) GetAuthorizedAuditLogsOffset(ctx context.Context, arg GetAuditLogsOffsetParams, prepared rbac.PreparedAuthorized) ([]GetAuditLogsOffsetRow, error) {
	authorizedFilter, err := prepared.CompileToSQL(ctx, regosql.ConvertConfig{
		VariableConverter: regosql.AuditLogConverter(),
	})
	if err != nil {
		return nil, xerrors.Errorf("compile authorized filter: %w", err)
	}

	filtered, err := insertAuthorizedFilter(getAuditLogsOffset, fmt.Sprintf(" AND %s", authorizedFilter))
	if err != nil {
		return nil, xerrors.Errorf("insert authorized filter: %w", err)
	}

	query := fmt.Sprintf("-- name: GetAuthorizedAuditLogsOffset :many\n%s", filtered)
	rows, err := q.db.QueryContext(ctx, query,
		arg.ResourceType,
		arg.ResourceID,
		arg.OrganizationID,
		arg.ResourceTarget,
		arg.Action,
		arg.UserID,
		arg.Username,
		arg.Email,
		arg.DateFrom,
		arg.DateTo,
		arg.BuildReason,
		arg.RequestID,
		arg.OffsetOpt,
		arg.LimitOpt,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []GetAuditLogsOffsetRow
	for rows.Next() {
		var i GetAuditLogsOffsetRow
		if err := rows.Scan(
			&i.AuditLog.ID,
			&i.AuditLog.Time,
			&i.AuditLog.UserID,
			&i.AuditLog.OrganizationID,
			&i.AuditLog.Ip,
			&i.AuditLog.UserAgent,
			&i.AuditLog.ResourceType,
			&i.AuditLog.ResourceID,
			&i.AuditLog.ResourceTarget,
			&i.AuditLog.Action,
			&i.AuditLog.Diff,
			&i.AuditLog.StatusCode,
			&i.AuditLog.AdditionalFields,
			&i.AuditLog.RequestID,
			&i.AuditLog.ResourceIcon,
			&i.UserUsername,
			&i.UserName,
			&i.UserEmail,
			&i.UserCreatedAt,
			&i.UserUpdatedAt,
			&i.UserLastSeenAt,
			&i.UserStatus,
			&i.UserLoginType,
			&i.UserRoles,
			&i.UserAvatarUrl,
			&i.UserDeleted,
			&i.UserQuietHoursSchedule,
			&i.OrganizationName,
			&i.OrganizationDisplayName,
			&i.OrganizationIcon,
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

func insertAuthorizedFilter(query string, replaceWith string) (string, error) {
	if !strings.Contains(query, authorizedQueryPlaceholder) {
		return "", xerrors.Errorf("query does not contain authorized replace string, this is not an authorized query")
	}
	filtered := strings.Replace(query, authorizedQueryPlaceholder, replaceWith, 1)
	return filtered, nil
}

// UpdateUserLinkRawJSON is a custom query for unit testing. Do not ever expose this
func (q *sqlQuerier) UpdateUserLinkRawJSON(ctx context.Context, userID uuid.UUID, data json.RawMessage) error {
	_, err := q.sdb.ExecContext(ctx, "UPDATE user_links SET claims = $2 WHERE user_id = $1", userID, data)
	return err
}
