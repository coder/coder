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
	connectionLogQuerier
	aibridgeQuerier
	chatQuerier
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
		arg.ExactDisplayName,
		arg.FuzzyName,
		arg.FuzzyDisplayName,
		pq.Array(arg.IDs),
		arg.Deprecated,
		arg.HasAITask,
		arg.AuthorID,
		arg.AuthorUsername,
		arg.HasExternalAgent,
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
			&i.UseClassicParameterFlow,
			&i.CorsBehavior,
			&i.DisableModuleCache,
			&i.TimeTilAutostopNotify,
			&i.CreatedByAvatarURL,
			&i.CreatedByUsername,
			&i.CreatedByName,
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
		pq.Array(arg.HasAgentStatuses),
		arg.AgentInactiveDisconnectTimeoutSeconds,
		arg.Dormant,
		arg.LastUsedBefore,
		arg.LastUsedAfter,
		arg.UsingActive,
		arg.HasAITask,
		arg.HasExternalAgent,
		arg.Shared,
		arg.SharedWithUserID,
		arg.SharedWithGroupID,
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
			&i.GroupACL,
			&i.UserACL,
			&i.OwnerAvatarUrl,
			&i.OwnerUsername,
			&i.OwnerName,
			&i.OrganizationName,
			&i.OrganizationDisplayName,
			&i.OrganizationIcon,
			&i.OrganizationDescription,
			&i.TemplateName,
			&i.TemplateDisplayName,
			&i.TemplateIcon,
			&i.TemplateDescription,
			&i.TaskID,
			&i.GroupACLDisplayInfo,
			&i.UserACLDisplayInfo,
			&i.TemplateVersionID,
			&i.TemplateVersionName,
			&i.LatestBuildCompletedAt,
			&i.LatestBuildCanceledAt,
			&i.LatestBuildError,
			&i.LatestBuildTransition,
			&i.LatestBuildStatus,
			&i.LatestBuildHasExternalAgent,
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
		arg.Name,
		arg.ExactUsername,
		arg.ExactEmail,
		pq.Array(arg.Status),
		pq.Array(arg.RbacRole),
		arg.LastSeenBefore,
		arg.LastSeenAfter,
		arg.CreatedBefore,
		arg.CreatedAfter,
		arg.IncludeSystem,
		arg.GithubComUserID,
		pq.Array(arg.LoginType),
		arg.IsServiceAccount,
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
			&i.IsServiceAccount,
			&i.ChatSpendLimitMicros,
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
	CountAuthorizedAuditLogs(ctx context.Context, arg CountAuditLogsParams, prepared rbac.PreparedAuthorized) (int64, error)
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

func (q *sqlQuerier) CountAuthorizedAuditLogs(ctx context.Context, arg CountAuditLogsParams, prepared rbac.PreparedAuthorized) (int64, error) {
	authorizedFilter, err := prepared.CompileToSQL(ctx, regosql.ConvertConfig{
		VariableConverter: regosql.AuditLogConverter(),
	})
	if err != nil {
		return 0, xerrors.Errorf("compile authorized filter: %w", err)
	}

	filtered, err := insertAuthorizedFilter(countAuditLogs, fmt.Sprintf(" AND %s", authorizedFilter))
	if err != nil {
		return 0, xerrors.Errorf("insert authorized filter: %w", err)
	}

	query := fmt.Sprintf("-- name: CountAuthorizedAuditLogs :one\n%s", filtered)

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
		arg.CountCap,
	)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	var count int64
	for rows.Next() {
		if err := rows.Scan(&count); err != nil {
			return 0, err
		}
	}
	if err := rows.Close(); err != nil {
		return 0, err
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}
	return count, nil
}

type connectionLogQuerier interface {
	GetAuthorizedConnectionLogsOffset(ctx context.Context, arg GetConnectionLogsOffsetParams, prepared rbac.PreparedAuthorized) ([]GetConnectionLogsOffsetRow, error)
	CountAuthorizedConnectionLogs(ctx context.Context, arg CountConnectionLogsParams, prepared rbac.PreparedAuthorized) (int64, error)
}

func (q *sqlQuerier) GetAuthorizedConnectionLogsOffset(ctx context.Context, arg GetConnectionLogsOffsetParams, prepared rbac.PreparedAuthorized) ([]GetConnectionLogsOffsetRow, error) {
	authorizedFilter, err := prepared.CompileToSQL(ctx, regosql.ConvertConfig{
		VariableConverter: regosql.ConnectionLogConverter(),
	})
	if err != nil {
		return nil, xerrors.Errorf("compile authorized filter: %w", err)
	}
	filtered, err := insertAuthorizedFilter(getConnectionLogsOffset, fmt.Sprintf(" AND %s", authorizedFilter))
	if err != nil {
		return nil, xerrors.Errorf("insert authorized filter: %w", err)
	}

	query := fmt.Sprintf("-- name: GetAuthorizedConnectionLogsOffset :many\n%s", filtered)
	rows, err := q.db.QueryContext(ctx, query,
		arg.OrganizationID,
		arg.WorkspaceOwner,
		arg.WorkspaceOwnerID,
		arg.WorkspaceOwnerEmail,
		arg.Type,
		arg.UserID,
		arg.Username,
		arg.UserEmail,
		arg.ConnectedAfter,
		arg.ConnectedBefore,
		arg.WorkspaceID,
		arg.ConnectionID,
		arg.Status,
		arg.OffsetOpt,
		arg.LimitOpt,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []GetConnectionLogsOffsetRow
	for rows.Next() {
		var i GetConnectionLogsOffsetRow
		if err := rows.Scan(
			&i.ConnectionLog.ID,
			&i.ConnectionLog.ConnectTime,
			&i.ConnectionLog.OrganizationID,
			&i.ConnectionLog.WorkspaceOwnerID,
			&i.ConnectionLog.WorkspaceID,
			&i.ConnectionLog.WorkspaceName,
			&i.ConnectionLog.AgentName,
			&i.ConnectionLog.Type,
			&i.ConnectionLog.Ip,
			&i.ConnectionLog.Code,
			&i.ConnectionLog.UserAgent,
			&i.ConnectionLog.UserID,
			&i.ConnectionLog.SlugOrPort,
			&i.ConnectionLog.ConnectionID,
			&i.ConnectionLog.DisconnectTime,
			&i.ConnectionLog.DisconnectReason,
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
			&i.WorkspaceOwnerUsername,
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

func (q *sqlQuerier) CountAuthorizedConnectionLogs(ctx context.Context, arg CountConnectionLogsParams, prepared rbac.PreparedAuthorized) (int64, error) {
	authorizedFilter, err := prepared.CompileToSQL(ctx, regosql.ConvertConfig{
		VariableConverter: regosql.ConnectionLogConverter(),
	})
	if err != nil {
		return 0, xerrors.Errorf("compile authorized filter: %w", err)
	}
	filtered, err := insertAuthorizedFilter(countConnectionLogs, fmt.Sprintf(" AND %s", authorizedFilter))
	if err != nil {
		return 0, xerrors.Errorf("insert authorized filter: %w", err)
	}

	query := fmt.Sprintf("-- name: CountAuthorizedConnectionLogs :one\n%s", filtered)
	rows, err := q.db.QueryContext(ctx, query,
		arg.OrganizationID,
		arg.WorkspaceOwner,
		arg.WorkspaceOwnerID,
		arg.WorkspaceOwnerEmail,
		arg.Type,
		arg.UserID,
		arg.Username,
		arg.UserEmail,
		arg.ConnectedAfter,
		arg.ConnectedBefore,
		arg.WorkspaceID,
		arg.ConnectionID,
		arg.Status,
		arg.CountCap,
	)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	var count int64
	for rows.Next() {
		if err := rows.Scan(&count); err != nil {
			return 0, err
		}
	}
	if err := rows.Close(); err != nil {
		return 0, err
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}
	return count, nil
}

type chatQuerier interface {
	GetAuthorizedChats(ctx context.Context, arg GetChatsParams, prepared rbac.PreparedAuthorized) ([]GetChatsRow, error)
	GetAuthorizedChatsByChatFileID(ctx context.Context, fileID uuid.UUID, prepared rbac.PreparedAuthorized) ([]Chat, error)
}

func (q *sqlQuerier) GetAuthorizedChats(ctx context.Context, arg GetChatsParams, prepared rbac.PreparedAuthorized) ([]GetChatsRow, error) {
	if (arg.OwnedOnly || arg.SharedOnly) && arg.ViewerID == uuid.Nil {
		return nil, xerrors.New("viewer_id required when owned_only or shared_only is true")
	}
	if arg.SharedOnly && arg.SharedWithUserID == uuid.Nil && len(arg.SharedWithGroupIds) == 0 {
		return nil, xerrors.New("shared_with_user_id or shared_with_group_ids required when shared_only is true")
	}

	authorizedFilter, err := prepared.CompileToSQL(ctx, rbac.ConfigChats())
	if err != nil {
		return nil, xerrors.Errorf("compile authorized filter: %w", err)
	}

	filtered, err := insertAuthorizedFilter(getChats, fmt.Sprintf(" AND %s", authorizedFilter))
	if err != nil {
		return nil, xerrors.Errorf("insert authorized filter: %w", err)
	}

	// The name comment is for metric tracking
	query := fmt.Sprintf("-- name: GetAuthorizedChats :many\n%s", filtered)
	rows, err := q.db.QueryContext(ctx, query,
		arg.OwnedOnly,
		arg.SharedOnly,
		arg.ViewerID,
		arg.SharedWithUserID,
		pq.Array(arg.SharedWithGroupIds),
		arg.Archived,
		arg.AfterID,
		arg.LabelFilter,
		arg.DiffURL,
		arg.TitleQuery,
		arg.HasUnread,
		pq.Array(arg.PullRequestStatuses),
		arg.PrNumber,
		arg.RepoQuery,
		arg.PrTitleQuery,
		arg.Search,
		arg.OffsetOpt,
		arg.LimitOpt,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []GetChatsRow
	for rows.Next() {
		var i GetChatsRow
		if err := rows.Scan(
			&i.Chat.ID,
			&i.Chat.OwnerID,
			&i.Chat.WorkspaceID,
			&i.Chat.Title,
			&i.Chat.Status,
			&i.Chat.WorkerID,
			&i.Chat.StartedAt,
			&i.Chat.HeartbeatAt,
			&i.Chat.CreatedAt,
			&i.Chat.UpdatedAt,
			&i.Chat.ParentChatID,
			&i.Chat.RootChatID,
			&i.Chat.LastModelConfigID,
			&i.Chat.LastReasoningEffort,
			&i.Chat.Archived,
			&i.Chat.LastError,
			&i.Chat.Mode,
			pq.Array(&i.Chat.MCPServerIDs),
			&i.Chat.Labels,
			&i.Chat.BuildID,
			&i.Chat.AgentID,
			&i.Chat.PinOrder,
			&i.Chat.LastReadMessageID,
			&i.Chat.DynamicTools,
			&i.Chat.OrganizationID,
			&i.Chat.PlanMode,
			&i.Chat.ClientType,
			&i.Chat.LastTurnSummary,
			&i.Chat.Summary,
			&i.Chat.SummaryGeneratedAt,
			&i.Chat.SnapshotVersion,
			&i.Chat.HistoryVersion,
			&i.Chat.QueueVersion,
			&i.Chat.GenerationAttempt,
			&i.Chat.RetryState,
			&i.Chat.RetryStateVersion,
			&i.Chat.RunnerID,
			&i.Chat.RequiresActionDeadlineAt,
			&i.Chat.UserACL,
			&i.Chat.GroupACL,
			&i.Chat.OwnerUsername,
			&i.Chat.OwnerName,
			&i.Chat.ContextAggregateHash,
			&i.Chat.ContextDirtySince,
			&i.Chat.ContextDirtyResources,
			&i.Chat.ContextError,
			&i.Chat.CompactionRequestedAt,
			&i.HasUnread); err != nil {
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

func (q *sqlQuerier) GetAuthorizedChatsByChatFileID(ctx context.Context, fileID uuid.UUID, prepared rbac.PreparedAuthorized) ([]Chat, error) {
	authorizedFilter, err := prepared.CompileToSQL(ctx, rbac.ConfigChats())
	if err != nil {
		return nil, xerrors.Errorf("compile authorized filter: %w", err)
	}

	filtered, err := insertAuthorizedFilter(getChatsByChatFileID, fmt.Sprintf(" AND %s\nLIMIT 1", authorizedFilter))
	if err != nil {
		return nil, xerrors.Errorf("insert authorized filter: %w", err)
	}

	query := fmt.Sprintf("-- name: GetAuthorizedChatsByChatFileID :many\n%s", filtered)
	rows, err := q.db.QueryContext(ctx, query, fileID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []Chat
	for rows.Next() {
		var i Chat
		if err := rows.Scan(
			&i.ID,
			&i.OwnerID,
			&i.WorkspaceID,
			&i.Title,
			&i.Status,
			&i.WorkerID,
			&i.StartedAt,
			&i.HeartbeatAt,
			&i.CreatedAt,
			&i.UpdatedAt,
			&i.ParentChatID,
			&i.RootChatID,
			&i.LastModelConfigID,
			&i.LastReasoningEffort,
			&i.Archived,
			&i.LastError,
			&i.Mode,
			pq.Array(&i.MCPServerIDs),
			&i.Labels,
			&i.BuildID,
			&i.AgentID,
			&i.PinOrder,
			&i.LastReadMessageID,
			&i.DynamicTools,
			&i.OrganizationID,
			&i.PlanMode,
			&i.ClientType,
			&i.LastTurnSummary,
			&i.Summary,
			&i.SummaryGeneratedAt,
			&i.SnapshotVersion,
			&i.HistoryVersion,
			&i.QueueVersion,
			&i.GenerationAttempt,
			&i.RetryState,
			&i.RetryStateVersion,
			&i.RunnerID,
			&i.RequiresActionDeadlineAt,
			&i.UserACL,
			&i.GroupACL,
			&i.OwnerUsername,
			&i.OwnerName,
			&i.ContextAggregateHash,
			&i.ContextDirtySince,
			&i.ContextDirtyResources,
			&i.ContextError,
			&i.CompactionRequestedAt); err != nil {
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

type aibridgeQuerier interface {
	ListAuthorizedAIBridgeModels(ctx context.Context, arg ListAIBridgeModelsParams, prepared rbac.PreparedAuthorized) ([]string, error)
	ListAuthorizedAIBridgeClients(ctx context.Context, arg ListAIBridgeClientsParams, prepared rbac.PreparedAuthorized) ([]string, error)
	ListAuthorizedAIBridgeSessions(ctx context.Context, arg ListAIBridgeSessionsParams, prepared rbac.PreparedAuthorized) ([]ListAIBridgeSessionsRow, error)
	CountAuthorizedAIBridgeSessions(ctx context.Context, arg CountAIBridgeSessionsParams, prepared rbac.PreparedAuthorized) (int64, error)
	ListAuthorizedAIBridgeSessionThreads(ctx context.Context, arg ListAIBridgeSessionThreadsParams, prepared rbac.PreparedAuthorized) ([]ListAIBridgeSessionThreadsRow, error)
}

func (q *sqlQuerier) ListAuthorizedAIBridgeModels(ctx context.Context, arg ListAIBridgeModelsParams, prepared rbac.PreparedAuthorized) ([]string, error) {
	authorizedFilter, err := prepared.CompileToSQL(ctx, regosql.ConvertConfig{
		VariableConverter: regosql.AIBridgeInterceptionConverter(),
	})
	if err != nil {
		return nil, xerrors.Errorf("compile authorized filter: %w", err)
	}
	filtered, err := insertAuthorizedFilter(listAIBridgeModels, fmt.Sprintf(" AND %s", authorizedFilter))
	if err != nil {
		return nil, xerrors.Errorf("insert authorized filter: %w", err)
	}

	query := fmt.Sprintf("-- name: ListAIBridgeModels :many\n%s", filtered)
	rows, err := q.db.QueryContext(ctx, query, arg.Model, arg.Offset, arg.Limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []string
	for rows.Next() {
		var model string
		if err := rows.Scan(&model); err != nil {
			return nil, err
		}
		items = append(items, model)
	}
	return items, nil
}

func (q *sqlQuerier) ListAuthorizedAIBridgeClients(ctx context.Context, arg ListAIBridgeClientsParams, prepared rbac.PreparedAuthorized) ([]string, error) {
	authorizedFilter, err := prepared.CompileToSQL(ctx, regosql.ConvertConfig{
		VariableConverter: regosql.AIBridgeInterceptionConverter(),
	})
	if err != nil {
		return nil, xerrors.Errorf("compile authorized filter: %w", err)
	}
	filtered, err := insertAuthorizedFilter(listAIBridgeClients, fmt.Sprintf(" AND %s", authorizedFilter))
	if err != nil {
		return nil, xerrors.Errorf("insert authorized filter: %w", err)
	}

	query := fmt.Sprintf("-- name: ListAIBridgeClients :many\n%s", filtered)
	rows, err := q.db.QueryContext(ctx, query, arg.Client, arg.Offset, arg.Limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []string
	for rows.Next() {
		var client string
		if err := rows.Scan(&client); err != nil {
			return nil, err
		}
		items = append(items, client)
	}
	return items, nil
}

func (q *sqlQuerier) ListAuthorizedAIBridgeSessions(ctx context.Context, arg ListAIBridgeSessionsParams, prepared rbac.PreparedAuthorized) ([]ListAIBridgeSessionsRow, error) {
	authorizedFilter, err := prepared.CompileToSQL(ctx, regosql.ConvertConfig{
		VariableConverter: regosql.AIBridgeInterceptionConverter(),
	})
	if err != nil {
		return nil, xerrors.Errorf("compile authorized filter: %w", err)
	}
	filtered, err := insertAuthorizedFilter(listAIBridgeSessions, fmt.Sprintf(" AND %s", authorizedFilter))
	if err != nil {
		return nil, xerrors.Errorf("insert authorized filter: %w", err)
	}

	query := fmt.Sprintf("-- name: ListAuthorizedAIBridgeSessions :many\n%s", filtered)
	rows, err := q.db.QueryContext(ctx, query,
		arg.AfterSessionID,
		arg.StartedAfter,
		arg.StartedBefore,
		arg.InitiatorID,
		arg.Provider,
		arg.ProviderName,
		arg.Model,
		arg.Client,
		arg.SessionID,
		arg.Offset,
		arg.Limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []ListAIBridgeSessionsRow
	for rows.Next() {
		var i ListAIBridgeSessionsRow
		if err := rows.Scan(
			&i.SessionID,
			&i.UserID,
			&i.UserUsername,
			&i.UserName,
			&i.UserAvatarUrl,
			pq.Array(&i.Providers),
			pq.Array(&i.Models),
			&i.Client,
			&i.Metadata,
			&i.StartedAt,
			&i.EndedAt,
			&i.Threads,
			&i.InputTokens,
			&i.OutputTokens,
			&i.CacheReadInputTokens,
			&i.CacheWriteInputTokens,
			&i.LastPrompt,
			&i.LastActiveAt,
			&i.NetworkCallsTotal,
			&i.NetworkCallsBlocked,
			&i.FirewallActive,
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

func (q *sqlQuerier) CountAuthorizedAIBridgeSessions(ctx context.Context, arg CountAIBridgeSessionsParams, prepared rbac.PreparedAuthorized) (int64, error) {
	authorizedFilter, err := prepared.CompileToSQL(ctx, regosql.ConvertConfig{
		VariableConverter: regosql.AIBridgeInterceptionConverter(),
	})
	if err != nil {
		return 0, xerrors.Errorf("compile authorized filter: %w", err)
	}
	filtered, err := insertAuthorizedFilter(countAIBridgeSessions, fmt.Sprintf(" AND %s", authorizedFilter))
	if err != nil {
		return 0, xerrors.Errorf("insert authorized filter: %w", err)
	}

	query := fmt.Sprintf("-- name: CountAuthorizedAIBridgeSessions :one\n%s", filtered)
	rows, err := q.db.QueryContext(ctx, query,
		arg.StartedAfter,
		arg.StartedBefore,
		arg.InitiatorID,
		arg.Provider,
		arg.ProviderName,
		arg.Model,
		arg.Client,
		arg.SessionID,
	)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	var count int64
	for rows.Next() {
		if err := rows.Scan(&count); err != nil {
			return 0, err
		}
	}
	if err := rows.Close(); err != nil {
		return 0, err
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}
	return count, nil
}

func (q *sqlQuerier) ListAuthorizedAIBridgeSessionThreads(ctx context.Context, arg ListAIBridgeSessionThreadsParams, prepared rbac.PreparedAuthorized) ([]ListAIBridgeSessionThreadsRow, error) {
	authorizedFilter, err := prepared.CompileToSQL(ctx, regosql.ConvertConfig{
		VariableConverter: regosql.AIBridgeInterceptionConverter(),
	})
	if err != nil {
		return nil, xerrors.Errorf("compile authorized filter: %w", err)
	}
	filtered, err := insertAuthorizedFilter(listAIBridgeSessionThreads, fmt.Sprintf(" AND %s", authorizedFilter))
	if err != nil {
		return nil, xerrors.Errorf("insert authorized filter: %w", err)
	}

	query := fmt.Sprintf("-- name: ListAuthorizedAIBridgeSessionThreads :many\n%s", filtered)
	rows, err := q.db.QueryContext(ctx, query,
		arg.SessionID,
		arg.AfterID,
		arg.BeforeID,
		arg.Limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []ListAIBridgeSessionThreadsRow
	for rows.Next() {
		var i ListAIBridgeSessionThreadsRow
		if err := rows.Scan(
			&i.ThreadID,
			&i.AIBridgeInterception.ID,
			&i.AIBridgeInterception.InitiatorID,
			&i.AIBridgeInterception.Provider,
			&i.AIBridgeInterception.Model,
			&i.AIBridgeInterception.StartedAt,
			&i.AIBridgeInterception.Metadata,
			&i.AIBridgeInterception.EndedAt,
			&i.AIBridgeInterception.APIKeyID,
			&i.AIBridgeInterception.Client,
			&i.AIBridgeInterception.ThreadParentID,
			&i.AIBridgeInterception.ThreadRootID,
			&i.AIBridgeInterception.ClientSessionID,
			&i.AIBridgeInterception.SessionID,
			&i.AIBridgeInterception.ProviderName,
			&i.AIBridgeInterception.CredentialKind,
			&i.AIBridgeInterception.CredentialHint,
			&i.AIBridgeInterception.AgentFirewallSessionID,
			&i.AIBridgeInterception.AgentFirewallSequenceNumber,
			&i.AIBridgeInterception.ErrorType,
			&i.AIBridgeInterception.ErrorMessage,
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
	filtered := strings.ReplaceAll(query, authorizedQueryPlaceholder, replaceWith)
	return filtered, nil
}

// UpdateUserLinkRawJSON is a custom query for unit testing. Do not ever expose this
func (q *sqlQuerier) UpdateUserLinkRawJSON(ctx context.Context, userID uuid.UUID, data json.RawMessage) error {
	_, err := q.sdb.ExecContext(ctx, "UPDATE user_links SET claims = $2 WHERE user_id = $1", userID, data)
	return err
}
