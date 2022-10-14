package coderd

import (
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

	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/coderd/httpapi"
	"github.com/coder/coder/coderd/httpmw"
	"github.com/coder/coder/coderd/rbac"
	"github.com/coder/coder/codersdk"
)

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

	dblogs, err := api.Database.GetAuditLogsOffset(ctx, database.GetAuditLogsOffsetParams{
		Offset:       int32(page.Offset),
		Limit:        int32(page.Limit),
		ResourceType: filter.ResourceType,
		ResourceID:   filter.ResourceID,
		Action:       filter.Action,
		Username:     filter.Username,
		Email:        filter.Email,
	})
	if err != nil {
		httpapi.InternalServerError(rw, err)
		return
	}

	httpapi.Write(ctx, rw, http.StatusOK, codersdk.AuditLogResponse{
		AuditLogs: convertAuditLogs(dblogs),
	})
}

func (api *API) auditLogCount(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if !api.Authorize(r, rbac.ActionRead, rbac.ResourceAuditLog) {
		httpapi.Forbidden(rw)
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

	count, err := api.Database.GetAuditLogCount(ctx, database.GetAuditLogCountParams{
		ResourceType: filter.ResourceType,
		ResourceID:   filter.ResourceID,
		Action:       filter.Action,
		Username:     filter.Username,
		Email:        filter.Email,
	})
	if err != nil {
		httpapi.InternalServerError(rw, err)
		return
	}

	httpapi.Write(ctx, rw, http.StatusOK, codersdk.AuditLogCountResponse{
		Count: count,
	})
}

func (api *API) generateFakeAuditLog(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if !api.Authorize(r, rbac.ActionRead, rbac.ResourceAuditLog) {
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

	ipRaw, _, _ := net.SplitHostPort(r.RemoteAddr)
	ip := net.ParseIP(ipRaw)
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

	_, err = api.Database.InsertAuditLog(ctx, database.InsertAuditLogParams{
		ID:               uuid.New(),
		Time:             time.Now(),
		UserID:           user.ID,
		Ip:               ipNet,
		UserAgent:        r.UserAgent(),
		ResourceType:     database.ResourceType(params.ResourceType),
		ResourceID:       params.ResourceID,
		ResourceTarget:   user.Username,
		Action:           database.AuditAction(params.Action),
		Diff:             diff,
		StatusCode:       http.StatusOK,
		AdditionalFields: []byte("{}"),
	})
	if err != nil {
		httpapi.InternalServerError(rw, err)
		return
	}

	rw.WriteHeader(http.StatusNoContent)
}

func convertAuditLogs(dblogs []database.GetAuditLogsOffsetRow) []codersdk.AuditLog {
	alogs := make([]codersdk.AuditLog, 0, len(dblogs))

	for _, dblog := range dblogs {
		alogs = append(alogs, convertAuditLog(dblog))
	}

	return alogs
}

func convertAuditLog(dblog database.GetAuditLogsOffsetRow) codersdk.AuditLog {
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
			Status:    codersdk.UserStatus(dblog.UserStatus),
			Roles:     []codersdk.Role{},
			AvatarURL: dblog.UserAvatarUrl.String,
		}

		for _, roleName := range dblog.UserRoles {
			rbacRole, _ := rbac.RoleByName(roleName)
			user.Roles = append(user.Roles, convertRole(rbacRole))
		}
	}

	return codersdk.AuditLog{
		ID:               dblog.ID,
		RequestID:        dblog.RequestID,
		Time:             dblog.Time,
		OrganizationID:   dblog.OrganizationID,
		IP:               ip,
		UserAgent:        dblog.UserAgent,
		ResourceType:     codersdk.ResourceType(dblog.ResourceType),
		ResourceID:       dblog.ResourceID,
		ResourceTarget:   dblog.ResourceTarget,
		ResourceIcon:     dblog.ResourceIcon,
		Action:           codersdk.AuditAction(dblog.Action),
		Diff:             diff,
		StatusCode:       dblog.StatusCode,
		AdditionalFields: dblog.AdditionalFields,
		Description:      auditLogDescription(dblog),
		User:             user,
	}
}

func auditLogDescription(alog database.GetAuditLogsOffsetRow) string {
	str := fmt.Sprintf("{user} %s %s",
		codersdk.AuditAction(alog.Action).FriendlyString(),
		codersdk.ResourceType(alog.ResourceType).FriendlyString(),
	)

	// We don't display the name for git ssh keys. It's fairly long and doesn't
	// make too much sense to display.
	if alog.ResourceType != database.ResourceTypeGitSshKey {
		str += " {target}"
	}

	return str
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
	filter := database.GetAuditLogsOffsetParams{
		ResourceType: resourceTypeFromString(parser.String(searchParams, "", "resource_type")),
		ResourceID:   parser.UUID(searchParams, uuid.Nil, "resource_id"),
		Action:       actionFromString(parser.String(searchParams, "", "action")),
		Username:     parser.String(searchParams, "", "username"),
		Email:        parser.String(searchParams, "", "email"),
	}

	return filter, parser.Errors
}

func resourceTypeFromString(resourceTypeString string) string {
	switch codersdk.ResourceType(resourceTypeString) {
	case codersdk.ResourceTypeOrganization:
		return resourceTypeString
	case codersdk.ResourceTypeTemplate:
		return resourceTypeString
	case codersdk.ResourceTypeTemplateVersion:
		return resourceTypeString
	case codersdk.ResourceTypeUser:
		return resourceTypeString
	case codersdk.ResourceTypeWorkspace:
		return resourceTypeString
	case codersdk.ResourceTypeGitSSHKey:
		return resourceTypeString
	case codersdk.ResourceTypeAPIKey:
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
	default:
	}
	return ""
}
