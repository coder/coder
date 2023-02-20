package coderd

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/tabbed/pqtype"
	"golang.org/x/xerrors"

	"cdr.dev/slog"
	"github.com/coder/coder/coderd/audit"
	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/coderd/httpapi"
	"github.com/coder/coder/coderd/httpmw"
	"github.com/coder/coder/coderd/rbac"
	"github.com/coder/coder/codersdk"
)

// @Summary Get audit logs
// @ID get-audit-logs
// @Security CoderSessionToken
// @Produce json
// @Tags Audit
// @Param q query string true "Search query"
// @Param after_id query string false "After ID" format(uuid)
// @Param limit query int false "Page limit"
// @Param offset query int false "Page offset"
// @Success 200 {object} codersdk.AuditLogResponse
// @Router /audit [get]
func (api *API) auditLogs(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if !api.Authorize(r, rbac.ActionRead, rbac.ResourceAuditLog) {
		httpapi.Forbidden(rw)
		return
	}

	page, ok := parsePagination(rw, r)
	if !ok {
		return
	}

	queryStr := r.URL.Query().Get("q")
	filter, errs := auditSearchQuery(queryStr)
	if len(errs) > 0 {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message:     "Invalid audit search query.",
			Validations: errs,
		})
		return
	}
	filter.Offset = int32(page.Offset)
	filter.Limit = int32(page.Limit)

	dblogs, err := api.Database.GetAuditLogsOffset(ctx, filter)
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
func (api *API) generateFakeAuditLog(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if !api.Authorize(r, rbac.ActionCreate, rbac.ResourceAuditLog) {
		httpapi.Forbidden(rw)
		return
	}

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
	ip, _ := netip.AddrFromSlice(dblog.Ip.IPNet.IP)

	diff := codersdk.AuditDiff{}
	_ = json.Unmarshal(dblog.Diff, &diff)

	var user *codersdk.User

	if dblog.UserUsername.Valid {
		user = &codersdk.User{
			ID:        dblog.UserID,
			Username:  dblog.UserUsername.String,
			Email:     dblog.UserEmail.String,
			CreatedAt: dblog.UserCreatedAt.Time,
			Status:    codersdk.UserStatus(dblog.UserStatus.UserStatus),
			Roles:     []codersdk.Role{},
			AvatarURL: dblog.UserAvatarUrl.String,
		}

		for _, roleName := range dblog.UserRoles {
			rbacRole, _ := rbac.RoleByName(roleName)
			user.Roles = append(user.Roles, convertRole(rbacRole))
		}
	}

	var (
		additionalFieldsBytes = []byte(dblog.AdditionalFields)
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

		dblog.AdditionalFields, err = json.Marshal(resourceInfo)
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

	return codersdk.AuditLog{
		ID:               dblog.ID,
		RequestID:        dblog.RequestID,
		Time:             dblog.Time,
		OrganizationID:   dblog.OrganizationID,
		IP:               ip,
		UserAgent:        dblog.UserAgent.String,
		ResourceType:     codersdk.ResourceType(dblog.ResourceType),
		ResourceID:       dblog.ResourceID,
		ResourceTarget:   dblog.ResourceTarget,
		ResourceIcon:     dblog.ResourceIcon,
		Action:           codersdk.AuditAction(dblog.Action),
		Diff:             diff,
		StatusCode:       dblog.StatusCode,
		AdditionalFields: dblog.AdditionalFields,
		User:             user,
		Description:      auditLogDescription(dblog),
		ResourceLink:     resourceLink,
		IsDeleted:        isDeleted,
	}
}

func auditLogDescription(alog database.GetAuditLogsOffsetRow) string {
	str := fmt.Sprintf("{user} %s",
		codersdk.AuditAction(alog.Action).Friendly(),
	)

	// API Key resources do not have targets and follow the below format:
	// "User {logged in | logged out}"
	if alog.ResourceType == database.ResourceTypeApiKey {
		return str
	}

	// We don't display the name (target) for git ssh keys. It's fairly long and doesn't
	// make too much sense to display.
	if alog.ResourceType == database.ResourceTypeGitSshKey {
		str += fmt.Sprintf(" the %s",
			codersdk.ResourceType(alog.ResourceType).FriendlyString())
		return str
	}

	str += fmt.Sprintf(" %s",
		codersdk.ResourceType(alog.ResourceType).FriendlyString())

	str += " {target}"

	return str
}

func (api *API) auditLogIsResourceDeleted(ctx context.Context, alog database.GetAuditLogsOffsetRow) bool {
	switch alog.ResourceType {
	case database.ResourceTypeTemplate:
		template, err := api.Database.GetTemplateByID(ctx, alog.ResourceID)
		if err != nil {
			if xerrors.Is(err, sql.ErrNoRows) {
				return true
			}
			api.Logger.Error(ctx, "fetch template", slog.Error(err))
		}
		return template.Deleted
	case database.ResourceTypeUser:
		user, err := api.Database.GetUserByID(ctx, alog.ResourceID)
		if err != nil {
			if xerrors.Is(err, sql.ErrNoRows) {
				return true
			}
			api.Logger.Error(ctx, "fetch user", slog.Error(err))
		}
		return user.Deleted
	case database.ResourceTypeWorkspace:
		workspace, err := api.Database.GetWorkspaceByID(ctx, alog.ResourceID)
		if err != nil {
			if xerrors.Is(err, sql.ErrNoRows) {
				return true
			}
			api.Logger.Error(ctx, "fetch workspace", slog.Error(err))
		}
		return workspace.Deleted
	case database.ResourceTypeWorkspaceBuild:
		workspaceBuild, err := api.Database.GetWorkspaceBuildByID(ctx, alog.ResourceID)
		if err != nil {
			if xerrors.Is(err, sql.ErrNoRows) {
				return true
			}
			api.Logger.Error(ctx, "fetch workspace build", slog.Error(err))
		}
		// We use workspace as a proxy for workspace build here
		workspace, err := api.Database.GetWorkspaceByID(ctx, workspaceBuild.WorkspaceID)
		if err != nil {
			if xerrors.Is(err, sql.ErrNoRows) {
				return true
			}
			api.Logger.Error(ctx, "fetch workspace", slog.Error(err))
		}
		return workspace.Deleted
	default:
		return false
	}
}

func (api *API) auditLogResourceLink(ctx context.Context, alog database.GetAuditLogsOffsetRow, additionalFields audit.AdditionalFields) string {
	switch alog.ResourceType {
	case database.ResourceTypeTemplate:
		return fmt.Sprintf("/templates/%s",
			alog.ResourceTarget)

	case database.ResourceTypeUser:
		return fmt.Sprintf("/users?filter=%s",
			alog.ResourceTarget)

	case database.ResourceTypeWorkspace:
		workspace, getWorkspaceErr := api.Database.GetWorkspaceByID(ctx, alog.ResourceID)
		if getWorkspaceErr != nil {
			return ""
		}
		workspaceOwner, getWorkspaceOwnerErr := api.Database.GetUserByID(ctx, workspace.OwnerID)
		if getWorkspaceOwnerErr != nil {
			return ""
		}
		return fmt.Sprintf("/@%s/%s",
			workspaceOwner.Username, alog.ResourceTarget)

	case database.ResourceTypeWorkspaceBuild:
		if len(additionalFields.WorkspaceName) == 0 || len(additionalFields.BuildNumber) == 0 {
			return ""
		}
		workspaceBuild, getWorkspaceBuildErr := api.Database.GetWorkspaceBuildByID(ctx, alog.ResourceID)
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

	default:
		return ""
	}
}

// auditSearchQuery takes a query string and returns the auditLog filter.
// It also can return the list of validation errors to return to the api.
func auditSearchQuery(query string) (database.GetAuditLogsOffsetParams, []codersdk.ValidationError) {
	searchParams := make(url.Values)
	if query == "" {
		// No filter
		return database.GetAuditLogsOffsetParams{}, nil
	}
	query = strings.ToLower(query)
	// Because we do this in 2 passes, we want to maintain quotes on the first
	// pass.Further splitting occurs on the second pass and quotes will be
	// dropped.
	elements := splitQueryParameterByDelimiter(query, ' ', true)
	for _, element := range elements {
		parts := splitQueryParameterByDelimiter(element, ':', false)
		switch len(parts) {
		case 1:
			// No key:value pair.
			searchParams.Set("resource_type", parts[0])
		case 2:
			searchParams.Set(parts[0], parts[1])
		default:
			return database.GetAuditLogsOffsetParams{}, []codersdk.ValidationError{
				{Field: "q", Detail: fmt.Sprintf("Query element %q can only contain 1 ':'", element)},
			}
		}
	}

	// Using the query param parser here just returns consistent errors with
	// other parsing.
	parser := httpapi.NewQueryParamParser()
	const layout = "2006-01-02"

	var (
		dateFromString    = parser.String(searchParams, "", "date_from")
		dateToString      = parser.String(searchParams, "", "date_to")
		parsedDateFrom, _ = time.Parse(layout, dateFromString)
		parsedDateTo, _   = time.Parse(layout, dateToString)
	)

	if dateToString != "" {
		parsedDateTo = parsedDateTo.Add(23*time.Hour + 59*time.Minute + 59*time.Second) // parsedDateTo goes to 23:59
	}

	if dateToString != "" && parsedDateTo.Before(parsedDateFrom) {
		return database.GetAuditLogsOffsetParams{}, []codersdk.ValidationError{
			{Field: "q", Detail: fmt.Sprintf("DateTo value %q cannot be before than DateFrom", parsedDateTo)},
		}
	}

	filter := database.GetAuditLogsOffsetParams{
		ResourceType: resourceTypeFromString(parser.String(searchParams, "", "resource_type")),
		ResourceID:   parser.UUID(searchParams, uuid.Nil, "resource_id"),
		Action:       actionFromString(parser.String(searchParams, "", "action")),
		Username:     parser.String(searchParams, "", "username"),
		Email:        parser.String(searchParams, "", "email"),
		DateFrom:     parsedDateFrom,
		DateTo:       parsedDateTo,
		BuildReason:  buildReasonFromString(parser.String(searchParams, "", "build_reason")),
	}

	return filter, parser.Errors
}

func resourceTypeFromString(resourceTypeString string) string {
	switch codersdk.ResourceType(resourceTypeString) {
	case codersdk.ResourceTypeTemplate:
		return resourceTypeString
	case codersdk.ResourceTypeTemplateVersion:
		return resourceTypeString
	case codersdk.ResourceTypeUser:
		return resourceTypeString
	case codersdk.ResourceTypeWorkspace:
		return resourceTypeString
	case codersdk.ResourceTypeWorkspaceBuild:
		return resourceTypeString
	case codersdk.ResourceTypeGitSSHKey:
		return resourceTypeString
	case codersdk.ResourceTypeAPIKey:
		return resourceTypeString
	case codersdk.ResourceTypeGroup:
		return resourceTypeString
	case codersdk.ResourceTypeLicense:
		return resourceTypeString
	}
	return ""
}

func actionFromString(actionString string) string {
	switch codersdk.AuditAction(actionString) {
	case codersdk.AuditActionCreate:
		return actionString
	case codersdk.AuditActionWrite:
		return actionString
	case codersdk.AuditActionDelete:
		return actionString
	case codersdk.AuditActionStart:
		return actionString
	case codersdk.AuditActionStop:
		return actionString
	case codersdk.AuditActionLogin:
		return actionString
	case codersdk.AuditActionLogout:
		return actionString
	default:
	}
	return ""
}

func buildReasonFromString(buildReasonString string) string {
	switch codersdk.BuildReason(buildReasonString) {
	case codersdk.BuildReasonInitiator:
		return buildReasonString
	case codersdk.BuildReasonAutostart:
		return buildReasonString
	case codersdk.BuildReasonAutostop:
		return buildReasonString
	default:
	}
	return ""
}
