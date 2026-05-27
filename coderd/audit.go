package coderd

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/sqlc-dev/pqtype"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/audit"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/db2sdk"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/coderd/searchquery"
	"github.com/coder/coder/v2/codersdk"
)

// Limit the count query to avoid a slow sequential scan due to joins
// on a large table. Set to 0 to disable capping (but also see the note
// in the SQL query).
const auditLogCountCap = 2000

// @Summary Get audit logs
// @ID get-audit-logs
// @Security CoderSessionToken
// @Produce json
// @Tags Audit
// @Param q query string false "Search query"
// @Param limit query int true "Page limit"
// @Param offset query int false "Page offset"
// @Success 200 {object} codersdk.AuditLogResponse
// @Router /api/v2/audit [get]
func (api *API) auditLogs(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	apiKey := httpmw.APIKey(r)

	page, ok := ParsePagination(rw, r)
	if !ok {
		return
	}

	queryStr := r.URL.Query().Get("q")
	filter, countFilter, errs := searchquery.AuditLogs(ctx, api.Database, queryStr)
	if len(errs) > 0 {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message:     "Invalid audit search query.",
			Validations: errs,
		})
		return
	}
	// #nosec G115 - Safe conversion as pagination offset is expected to be within int32 range
	filter.OffsetOpt = int32(page.Offset)
	// #nosec G115 - Safe conversion as pagination limit is expected to be within int32 range
	filter.LimitOpt = int32(page.Limit)

	if filter.Username == "me" {
		filter.UserID = apiKey.UserID
		filter.Username = ""
		countFilter.UserID = apiKey.UserID
		countFilter.Username = ""
	}

	countFilter.CountCap = auditLogCountCap
	count, err := api.Database.CountAuditLogs(ctx, countFilter)
	if dbauthz.IsNotAuthorizedError(err) {
		httpapi.Forbidden(rw)
		return
	}
	if err != nil {
		httpapi.InternalServerError(rw, err)
		return
	}
	// If count is 0, then we don't need to query audit logs
	if count == 0 {
		httpapi.Write(ctx, rw, http.StatusOK, codersdk.AuditLogResponse{
			AuditLogs: []codersdk.AuditLog{},
			Count:     0,
			CountCap:  auditLogCountCap,
		})
		return
	}

	dblogs, err := api.Database.GetAuditLogsOffset(ctx, filter)
	if dbauthz.IsNotAuthorizedError(err) {
		httpapi.Forbidden(rw)
		return
	}
	if err != nil {
		httpapi.InternalServerError(rw, err)
		return
	}

	httpapi.Write(ctx, rw, http.StatusOK, codersdk.AuditLogResponse{
		AuditLogs: api.convertAuditLogs(ctx, dblogs),
		Count:     count,
		CountCap:  auditLogCountCap,
	})
}

// @Summary Generate fake audit log
// @ID generate-fake-audit-log
// @Security CoderSessionToken
// @Accept json
// @Tags Audit
// @Param request body codersdk.CreateTestAuditLogRequest true "Audit log request"
// @Success 204
// @Router /api/v2/audit/testgenerate [post]
// @x-apidocgen {"skip": true}
func (api *API) generateFakeAuditLog(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	key := httpmw.APIKey(r)
	user, err := api.Database.GetUserByID(ctx, key.UserID)
	if err != nil {
		httpapi.InternalServerError(rw, err)
		return
	}

	diff, err := json.Marshal(codersdk.AuditDiff{
		"foo": codersdk.AuditDiffField{Old: "bar", New: "baz"},
	})
	if err != nil {
		httpapi.InternalServerError(rw, err)
		return
	}

	ip := net.ParseIP(r.RemoteAddr)
	ipNet := pqtype.Inet{}
	if ip != nil {
		ipNet = pqtype.Inet{
			IPNet: net.IPNet{
				IP:   ip,
				Mask: net.CIDRMask(len(ip)*8, len(ip)*8),
			},
			Valid: true,
		}
	}

	var params codersdk.CreateTestAuditLogRequest
	if !httpapi.Read(ctx, rw, r, &params) {
		return
	}
	if params.Action == "" {
		params.Action = codersdk.AuditActionWrite
	}
	if params.ResourceType == "" {
		params.ResourceType = codersdk.ResourceTypeUser
	}
	if params.ResourceID == uuid.Nil {
		params.ResourceID = uuid.New()
	}
	if params.Time.IsZero() {
		params.Time = time.Now()
	}
	if len(params.AdditionalFields) == 0 {
		params.AdditionalFields = json.RawMessage("{}")
	}

	_, err = api.Database.InsertAuditLog(ctx, database.InsertAuditLogParams{
		ID:               uuid.New(),
		Time:             params.Time,
		UserID:           user.ID,
		Ip:               ipNet,
		UserAgent:        sql.NullString{String: r.UserAgent(), Valid: true},
		ResourceType:     database.ResourceType(params.ResourceType),
		ResourceID:       params.ResourceID,
		ResourceTarget:   user.Username,
		Action:           database.AuditAction(params.Action),
		Diff:             diff,
		StatusCode:       http.StatusOK,
		AdditionalFields: params.AdditionalFields,
		RequestID:        params.RequestID,
		ResourceIcon:     "",
		OrganizationID:   params.OrganizationID,
	})
	if err != nil {
		httpapi.InternalServerError(rw, err)
		return
	}

	rw.WriteHeader(http.StatusNoContent)
}

func (api *API) convertAuditLogs(ctx context.Context, dblogs []database.GetAuditLogsOffsetRow) []codersdk.AuditLog {
	alogs := make([]codersdk.AuditLog, 0, len(dblogs))

	for _, dblog := range dblogs {
		alogs = append(alogs, api.convertAuditLog(ctx, dblog))
	}

	return alogs
}

func (api *API) convertAuditLog(ctx context.Context, dblog database.GetAuditLogsOffsetRow) codersdk.AuditLog {
	ip, _ := netip.AddrFromSlice(dblog.AuditLog.Ip.IPNet.IP)

	diff := codersdk.AuditDiff{}
	_ = json.Unmarshal(dblog.AuditLog.Diff, &diff)

	var user *codersdk.User
	if dblog.UserUsername.Valid {
		// Leaving the organization IDs blank for now; not sure they are useful for
		// the audit query anyway?
		sdkUser := db2sdk.User(database.User{
			ID:                 dblog.AuditLog.UserID,
			Email:              dblog.UserEmail.String,
			Username:           dblog.UserUsername.String,
			CreatedAt:          dblog.UserCreatedAt.Time,
			UpdatedAt:          dblog.UserUpdatedAt.Time,
			Status:             dblog.UserStatus.UserStatus,
			RBACRoles:          dblog.UserRoles,
			LoginType:          dblog.UserLoginType.LoginType,
			AvatarURL:          dblog.UserAvatarUrl.String,
			Deleted:            dblog.UserDeleted.Bool,
			LastSeenAt:         dblog.UserLastSeenAt.Time,
			QuietHoursSchedule: dblog.UserQuietHoursSchedule.String,
			Name:               dblog.UserName.String,
		}, []uuid.UUID{})
		user = &sdkUser
	}

	var (
		additionalFieldsBytes = []byte(dblog.AuditLog.AdditionalFields)
		additionalFields      audit.AdditionalFields
		err                   = json.Unmarshal(additionalFieldsBytes, &additionalFields)
	)
	if err != nil {
		api.Logger.Error(ctx, "unmarshal additional fields", slog.Error(err))
		resourceInfo := audit.AdditionalFields{
			WorkspaceName:  "unknown",
			BuildNumber:    "unknown",
			BuildReason:    "unknown",
			WorkspaceOwner: "unknown",
		}

		dblog.AuditLog.AdditionalFields, err = json.Marshal(resourceInfo)
		api.Logger.Error(ctx, "marshal additional fields", slog.Error(err))
	}

	var (
		isDeleted    = api.auditLogIsResourceDeleted(ctx, dblog)
		resourceLink string
	)
	if isDeleted {
		resourceLink = ""
	} else {
		resourceLink = api.auditLogResourceLink(ctx, dblog, additionalFields)
	}

	alog := codersdk.AuditLog{
		ID:        dblog.AuditLog.ID,
		RequestID: dblog.AuditLog.RequestID,
		Time:      dblog.AuditLog.Time,
		// OrganizationID is deprecated.
		OrganizationID:   dblog.AuditLog.OrganizationID,
		IP:               ip,
		UserAgent:        dblog.AuditLog.UserAgent.String,
		ResourceType:     codersdk.ResourceType(dblog.AuditLog.ResourceType),
		ResourceID:       dblog.AuditLog.ResourceID,
		ResourceTarget:   dblog.AuditLog.ResourceTarget,
		ResourceIcon:     dblog.AuditLog.ResourceIcon,
		Action:           codersdk.AuditAction(dblog.AuditLog.Action),
		Diff:             diff,
		StatusCode:       dblog.AuditLog.StatusCode,
		AdditionalFields: dblog.AuditLog.AdditionalFields,
		User:             user,
		Description:      auditLogDescription(dblog),
		ResourceLink:     resourceLink,
		IsDeleted:        isDeleted,
	}

	if dblog.AuditLog.OrganizationID != uuid.Nil {
		alog.Organization = &codersdk.MinimalOrganization{
			ID:          dblog.AuditLog.OrganizationID,
			Name:        dblog.OrganizationName,
			DisplayName: dblog.OrganizationDisplayName,
			Icon:        dblog.OrganizationIcon,
		}
	}

	return alog
}

func auditLogDescription(alog database.GetAuditLogsOffsetRow) string {
	b := strings.Builder{}

	// NOTE: WriteString always returns a nil error, so we never check it

	// Requesting a password reset can be performed by anyone that knows the email
	// of a user so saying the user performed this action might be slightly misleading.
	if alog.AuditLog.Action != database.AuditActionRequestPasswordReset {
		_, _ = b.WriteString("{user} ")
	}

	if chatDescription, ok := chatAuditLogDescription(alog); ok {
		_, _ = b.WriteString(chatDescription)
		return b.String()
	}

	switch {
	case alog.AuditLog.StatusCode == int32(http.StatusSeeOther):
		_, _ = b.WriteString("was redirected attempting to ")
		_, _ = b.WriteString(string(alog.AuditLog.Action))
	case alog.AuditLog.StatusCode >= 400:
		_, _ = b.WriteString("unsuccessfully attempted to ")
		_, _ = b.WriteString(string(alog.AuditLog.Action))
	default:
		_, _ = b.WriteString(codersdk.AuditAction(alog.AuditLog.Action).Friendly())
	}

	// API Key resources (used for authentication) do not have targets and follow the below format:
	// "User {logged in | logged out | registered}"
	if alog.AuditLog.ResourceType == database.ResourceTypeApiKey &&
		(alog.AuditLog.Action == database.AuditActionLogin || alog.AuditLog.Action == database.AuditActionLogout || alog.AuditLog.Action == database.AuditActionRegister) {
		return b.String()
	}

	// We don't display the name (target) for git ssh keys. It's fairly long and doesn't
	// make too much sense to display.
	if alog.AuditLog.ResourceType == database.ResourceTypeGitSshKey {
		_, _ = b.WriteString(" the ")
		_, _ = b.WriteString(codersdk.ResourceType(alog.AuditLog.ResourceType).FriendlyString())
		return b.String()
	}

	if alog.AuditLog.Action == database.AuditActionRequestPasswordReset {
		_, _ = b.WriteString(" for")
	} else {
		_, _ = b.WriteString(" ")
		_, _ = b.WriteString(codersdk.ResourceType(alog.AuditLog.ResourceType).FriendlyString())
	}

	if alog.AuditLog.ResourceType == database.ResourceTypeConvertLogin {
		_, _ = b.WriteString(" to")
	}

	_, _ = b.WriteString(" {target}")

	return b.String()
}

func chatAuditLogDescription(alog database.GetAuditLogsOffsetRow) (string, bool) {
	if alog.AuditLog.ResourceType != database.ResourceTypeChat ||
		alog.AuditLog.Action != database.AuditActionWrite ||
		alog.AuditLog.StatusCode >= 400 ||
		alog.AuditLog.StatusCode == int32(http.StatusSeeOther) {
		return "", false
	}

	diff := codersdk.AuditDiff{}
	if err := json.Unmarshal(alog.AuditLog.Diff, &diff); err != nil {
		return "", false
	}

	if archived, ok := diff["archived"]; ok && len(diff) == 1 {
		oldArchived, oldOK := auditDiffBool(archived.Old)
		newArchived, newOK := auditDiffBool(archived.New)
		if !oldOK || !newOK {
			return "", false
		}
		if !oldArchived && newArchived {
			return "archived chat {target}", true
		}
		if oldArchived && !newArchived {
			return "unarchived chat {target}", true
		}
		return "", false
	}

	aclDiff, ok := auditLogChatACLChanges(diff)
	if !ok {
		return "", false
	}

	switch {
	case aclDiff.added > 0 && aclDiff.removed == 0 && !aclDiff.updated:
		return "shared chat {target} with " + auditLogChatACLTarget(aclDiff.addedUsers, aclDiff.addedGroups), true
	case aclDiff.added == 0 && aclDiff.removed > 0 && !aclDiff.updated:
		return "unshared chat {target} with " + auditLogChatACLTarget(aclDiff.removedUsers, aclDiff.removedGroups), true
	default:
		return "updated sharing for chat {target}", true
	}
}

type auditLogChatACLChange struct {
	added         int
	removed       int
	updated       bool
	addedUsers    int
	addedGroups   int
	removedUsers  int
	removedGroups int
}

func auditLogChatACLChanges(diff codersdk.AuditDiff) (auditLogChatACLChange, bool) {
	if len(diff) == 0 || len(diff) > 2 {
		return auditLogChatACLChange{}, false
	}

	var change auditLogChatACLChange
	for field, diffField := range diff {
		isUserACL := field == "user_acl"
		isGroupACL := field == "group_acl"
		if !isUserACL && !isGroupACL {
			return auditLogChatACLChange{}, false
		}

		aclChange, ok := auditLogChatACLFieldChanges(diffField)
		if !ok {
			return auditLogChatACLChange{}, false
		}
		change.added += aclChange.added
		change.removed += aclChange.removed
		change.updated = change.updated || aclChange.updated
		if isUserACL {
			change.addedUsers += aclChange.added
			change.removedUsers += aclChange.removed
		} else {
			change.addedGroups += aclChange.added
			change.removedGroups += aclChange.removed
		}
	}

	if change.added == 0 && change.removed == 0 && !change.updated {
		return auditLogChatACLChange{}, false
	}
	return change, true
}

func auditLogChatACLFieldChanges(diffField codersdk.AuditDiffField) (auditLogChatACLChange, bool) {
	if diffField.Secret {
		return auditLogChatACLChange{}, false
	}

	oldACL, ok := auditDiffChatACL(diffField.Old)
	if !ok {
		return auditLogChatACLChange{}, false
	}
	newACL, ok := auditDiffChatACL(diffField.New)
	if !ok {
		return auditLogChatACLChange{}, false
	}

	var change auditLogChatACLChange
	for id, newEntry := range newACL {
		oldEntry, exists := oldACL[id]
		if !exists {
			change.added++
			continue
		}
		if !auditLogChatACLEqual(oldEntry, newEntry) {
			change.updated = true
		}
	}
	for id := range oldACL {
		if _, exists := newACL[id]; !exists {
			change.removed++
		}
	}
	return change, true
}

func auditDiffChatACL(value any) (map[string]database.ChatACLEntry, bool) {
	if value == nil {
		return map[string]database.ChatACLEntry{}, true
	}

	raw, err := json.Marshal(value)
	if err != nil {
		return nil, false
	}
	acl := map[string]database.ChatACLEntry{}
	if err := json.Unmarshal(raw, &acl); err != nil {
		return nil, false
	}
	return acl, true
}

func auditDiffBool(value any) (boolValue bool, ok bool) {
	boolValue, ok = value.(bool)
	return boolValue, ok
}

func auditLogChatACLEqual(a, b database.ChatACLEntry) bool {
	return slices.Equal(a.Permissions, b.Permissions)
}

func auditLogChatACLTarget(users, groups int) string {
	segments := make([]string, 0, 2)
	if users > 0 {
		segments = append(segments, auditLogCountSegment(users, "user"))
	}
	if groups > 0 {
		segments = append(segments, auditLogCountSegment(groups, "group"))
	}
	return strings.Join(segments, " and ")
}

func auditLogCountSegment(count int, singular string) string {
	plural := singular
	if count != 1 {
		plural += "s"
	}
	return fmt.Sprintf("%d %s", count, plural)
}

func (api *API) auditLogIsResourceDeleted(ctx context.Context, alog database.GetAuditLogsOffsetRow) bool {
	switch alog.AuditLog.ResourceType {
	case database.ResourceTypeTemplate:
		template, err := api.Database.GetTemplateByID(ctx, alog.AuditLog.ResourceID)
		if err != nil {
			if xerrors.Is(err, sql.ErrNoRows) {
				return true
			}
			api.Logger.Error(ctx, "unable to fetch template", slog.Error(err))
		}
		return template.Deleted
	case database.ResourceTypeUser:
		user, err := api.Database.GetUserByID(ctx, alog.AuditLog.ResourceID)
		if err != nil {
			if xerrors.Is(err, sql.ErrNoRows) {
				return true
			}
			api.Logger.Error(ctx, "unable to fetch user", slog.Error(err))
		}
		return user.Deleted
	case database.ResourceTypeWorkspace:
		workspace, err := api.Database.GetWorkspaceByID(ctx, alog.AuditLog.ResourceID)
		if err != nil {
			if xerrors.Is(err, sql.ErrNoRows) {
				return true
			}
			api.Logger.Error(ctx, "unable to fetch workspace", slog.Error(err))
		}
		return workspace.Deleted
	case database.ResourceTypeWorkspaceBuild:
		workspaceBuild, err := api.Database.GetWorkspaceBuildByID(ctx, alog.AuditLog.ResourceID)
		if err != nil {
			if xerrors.Is(err, sql.ErrNoRows) {
				return true
			}
			api.Logger.Error(ctx, "unable to fetch workspace build", slog.Error(err))
		}
		// We use workspace as a proxy for workspace build here
		workspace, err := api.Database.GetWorkspaceByID(ctx, workspaceBuild.WorkspaceID)
		if err != nil {
			if xerrors.Is(err, sql.ErrNoRows) {
				return true
			}
			api.Logger.Error(ctx, "unable to fetch workspace", slog.Error(err))
		}
		return workspace.Deleted
	case database.ResourceTypeWorkspaceAgent:
		// We use workspace as a proxy for workspace agents.
		workspace, err := api.Database.GetWorkspaceByAgentID(ctx, alog.AuditLog.ResourceID)
		if err != nil {
			if xerrors.Is(err, sql.ErrNoRows) {
				return true
			}
			api.Logger.Error(ctx, "unable to fetch workspace", slog.Error(err))
		}
		return workspace.Deleted
	case database.ResourceTypeWorkspaceApp:
		// We use workspace as a proxy for workspace apps.
		workspace, err := api.Database.GetWorkspaceByWorkspaceAppID(ctx, alog.AuditLog.ResourceID)
		if err != nil {
			if xerrors.Is(err, sql.ErrNoRows) {
				return true
			}
			api.Logger.Error(ctx, "unable to fetch workspace", slog.Error(err))
		}
		return workspace.Deleted
	case database.ResourceTypeOauth2ProviderApp:
		_, err := api.Database.GetOAuth2ProviderAppByID(ctx, alog.AuditLog.ResourceID)
		if xerrors.Is(err, sql.ErrNoRows) {
			return true
		} else if err != nil {
			api.Logger.Error(ctx, "unable to fetch oauth2 app", slog.Error(err))
		}
		return false
	case database.ResourceTypeOauth2ProviderAppSecret:
		_, err := api.Database.GetOAuth2ProviderAppSecretByID(ctx, alog.AuditLog.ResourceID)
		if xerrors.Is(err, sql.ErrNoRows) {
			return true
		} else if err != nil {
			api.Logger.Error(ctx, "unable to fetch oauth2 app secret", slog.Error(err))
		}
		return false
	case database.ResourceTypeTask:
		task, err := api.Database.GetTaskByID(ctx, alog.AuditLog.ResourceID)
		if xerrors.Is(err, sql.ErrNoRows) {
			return true
		} else if err != nil {
			api.Logger.Error(ctx, "unable to fetch task", slog.Error(err))
		}
		return task.DeletedAt.Valid && task.DeletedAt.Time.Before(time.Now())
	case database.ResourceTypeChat:
		// Chats are hard-deleted, so a 404 means deleted.
		_, err := api.Database.GetChatByID(ctx, alog.AuditLog.ResourceID)
		if xerrors.Is(err, sql.ErrNoRows) {
			return true
		}
		if err != nil {
			api.Logger.Error(ctx, "unable to fetch chat", slog.Error(err))
		}
		return false
	case database.ResourceTypeUserSecret:
		_, err := api.Database.GetUserSecretByID(ctx, alog.AuditLog.ResourceID)
		if xerrors.Is(err, sql.ErrNoRows) {
			return true
		}
		// Only users have user_secret:read on their own secrets. If dbauthz returns
		// ErrUnauthorized, it's not an error worth logging because we have enough
		// information to know it's not deleted.
		if err != nil && !dbauthz.IsNotAuthorizedError(err) {
			api.Logger.Error(ctx, "unable to fetch user secret", slog.Error(err))
		}
		return false
	default:
		return false
	}
}

func (api *API) auditLogResourceLink(ctx context.Context, alog database.GetAuditLogsOffsetRow, additionalFields audit.AdditionalFields) string {
	switch alog.AuditLog.ResourceType {
	case database.ResourceTypeTemplate:
		return fmt.Sprintf("/templates/%s",
			alog.AuditLog.ResourceTarget)

	case database.ResourceTypeUser:
		return fmt.Sprintf("/users?filter=%s",
			alog.AuditLog.ResourceTarget)

	case database.ResourceTypeWorkspace:
		workspace, getWorkspaceErr := api.Database.GetWorkspaceByID(ctx, alog.AuditLog.ResourceID)
		if getWorkspaceErr != nil {
			return ""
		}
		workspaceOwner, getWorkspaceOwnerErr := api.Database.GetUserByID(ctx, workspace.OwnerID)
		if getWorkspaceOwnerErr != nil {
			return ""
		}
		return fmt.Sprintf("/@%s/%s",
			workspaceOwner.Username, alog.AuditLog.ResourceTarget)

	case database.ResourceTypeWorkspaceBuild:
		if len(additionalFields.WorkspaceName) == 0 || len(additionalFields.BuildNumber) == 0 {
			return ""
		}
		workspaceBuild, getWorkspaceBuildErr := api.Database.GetWorkspaceBuildByID(ctx, alog.AuditLog.ResourceID)
		if getWorkspaceBuildErr != nil {
			return ""
		}
		workspace, getWorkspaceErr := api.Database.GetWorkspaceByID(ctx, workspaceBuild.WorkspaceID)
		if getWorkspaceErr != nil {
			return ""
		}
		workspaceOwner, getWorkspaceOwnerErr := api.Database.GetUserByID(ctx, workspace.OwnerID)
		if getWorkspaceOwnerErr != nil {
			return ""
		}
		return fmt.Sprintf("/@%s/%s/builds/%s",
			workspaceOwner.Username, additionalFields.WorkspaceName, additionalFields.BuildNumber)

	case database.ResourceTypeWorkspaceAgent:
		if additionalFields.WorkspaceOwner != "" && additionalFields.WorkspaceName != "" {
			return fmt.Sprintf("/@%s/%s", additionalFields.WorkspaceOwner, additionalFields.WorkspaceName)
		}
		workspace, getWorkspaceErr := api.Database.GetWorkspaceByAgentID(ctx, alog.AuditLog.ResourceID)
		if getWorkspaceErr != nil {
			return ""
		}
		return fmt.Sprintf("/@%s/%s", workspace.OwnerName, workspace.Name)

	case database.ResourceTypeWorkspaceApp:
		if additionalFields.WorkspaceOwner != "" && additionalFields.WorkspaceName != "" {
			return fmt.Sprintf("/@%s/%s", additionalFields.WorkspaceOwner, additionalFields.WorkspaceName)
		}
		workspace, getWorkspaceErr := api.Database.GetWorkspaceByWorkspaceAppID(ctx, alog.AuditLog.ResourceID)
		if getWorkspaceErr != nil {
			return ""
		}
		return fmt.Sprintf("/@%s/%s", workspace.OwnerName, workspace.Name)

	case database.ResourceTypeOauth2ProviderApp:
		return fmt.Sprintf("/deployment/oauth2-provider/apps/%s", alog.AuditLog.ResourceID)

	case database.ResourceTypeOauth2ProviderAppSecret:
		secret, err := api.Database.GetOAuth2ProviderAppSecretByID(ctx, alog.AuditLog.ResourceID)
		if err != nil {
			return ""
		}
		return fmt.Sprintf("/deployment/oauth2-provider/apps/%s", secret.AppID)

	case database.ResourceTypeTask:
		task, err := api.Database.GetTaskByID(ctx, alog.AuditLog.ResourceID)
		if err != nil {
			return ""
		}
		user, err := api.Database.GetUserByID(ctx, task.OwnerID)
		if err != nil {
			return ""
		}
		return fmt.Sprintf("/tasks/%s/%s", user.Username, task.ID)

	case database.ResourceTypeChat:
		// Chats are surfaced at /agents/{id}. They are owner-scoped but
		// not username-scoped in the URL like workspaces or tasks.
		return fmt.Sprintf("/agents/%s", alog.AuditLog.ResourceID)
	case database.ResourceTypeUserSecret:
		// TODO(PLAT-102): point at the user secrets management page once
		// it ships. Until then, the audit row links nowhere.
		return ""
	case database.ResourceTypeGroupAiBudget:
		// The resource_id is the group's UUID; link to the group's
		// settings page.
		group, err := api.Database.GetGroupByID(ctx, alog.AuditLog.ResourceID)
		if err != nil {
			return ""
		}
		org, err := api.Database.GetOrganizationByID(ctx, group.OrganizationID)
		if err != nil {
			return ""
		}
		return fmt.Sprintf("/organizations/%s/groups/%s", org.Name, group.Name)
	default:
		return ""
	}
}
