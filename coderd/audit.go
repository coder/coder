package coderd

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/sqlc-dev/pqtype"
	"golang.org/x/xerrors"

	"cdr.dev/slog"
	"github.com/coder/coder/v2/coderd/audit"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/db2sdk"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/coderd/searchquery"
	"github.com/coder/coder/v2/codersdk"
)

// @Summary Get audit logs
// @ID get-audit-logs
// @Security CoderSessionToken
// @Produce json
// @Tags Audit
// @Param q query string false "Search query"
// @Param limit query int true "Page limit"
// @Param offset query int false "Page offset"
// @Success 200 {object} codersdk.AuditLogResponse
// @Router /audit [get]
func (api *API) auditLogs(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	apiKey := httpmw.APIKey(r)

	page, ok := parsePagination(rw, r)
	if !ok {
		return
	}

	queryStr := r.URL.Query().Get("q")
	filter, errs := searchquery.AuditLogs(ctx, api.Database, queryStr)
	if len(errs) > 0 {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message:     "Invalid audit search query.",
			Validations: errs,
		})
		return
	}
	filter.OffsetOpt = int32(page.Offset)
	filter.LimitOpt = int32(page.Limit)

	if filter.Username == "me" {
		filter.UserID = apiKey.UserID
		filter.Username = ""
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
	// GetAuditLogsOffset does not return ErrNoRows because it uses a window function to get the count.
	// So we need to check if the dblogs is empty and return an empty array if so.
	if len(dblogs) == 0 {
		httpapi.Write(ctx, rw, http.StatusOK, codersdk.AuditLogResponse{
			AuditLogs: []codersdk.AuditLog{},
			Count:     0,
		})
		return
	}

	httpapi.Write(ctx, rw, http.StatusOK, codersdk.AuditLogResponse{
		AuditLogs: api.convertAuditLogs(ctx, dblogs),
		Count:     dblogs[0].Count,
	})
}

// @Summary Generate fake audit log
// @ID generate-fake-audit-log
// @Security CoderSessionToken
// @Accept json
// @Tags Audit
// @Param request body codersdk.CreateTestAuditLogRequest true "Audit log request"
// @Success 204
// @Router /audit/testgenerate [post]
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
		RequestID:        uuid.Nil, // no request ID to attach this to
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
			ThemePreference:    dblog.UserThemePreference.String,
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

	if alog.AuditLog.StatusCode >= 400 {
		_, _ = b.WriteString("unsuccessfully attempted to ")
		_, _ = b.WriteString(string(alog.AuditLog.Action))
	} else {
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

	case database.ResourceTypeOauth2ProviderApp:
		return fmt.Sprintf("/deployment/oauth2-provider/apps/%s", alog.AuditLog.ResourceID)

	case database.ResourceTypeOauth2ProviderAppSecret:
		secret, err := api.Database.GetOAuth2ProviderAppSecretByID(ctx, alog.AuditLog.ResourceID)
		if err != nil {
			return ""
		}
		return fmt.Sprintf("/deployment/oauth2-provider/apps/%s", secret.AppID)

	default:
		return ""
	}
}
