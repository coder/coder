package coderd

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/netip"
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
	if !api.Authorize(r, rbac.ActionRead, rbac.ResourceAuditLog) {
		httpapi.Forbidden(rw)
		return
	}

	ctx := r.Context()
	page, ok := parsePagination(rw, r)
	if !ok {
		return
	}

	dblogs, err := api.Database.GetAuditLogsOffset(ctx, database.GetAuditLogsOffsetParams{
		Offset: int32(page.Offset),
		Limit:  int32(page.Limit),
	})
	if err != nil {
		httpapi.InternalServerError(rw, err)
		return
	}

	httpapi.Write(rw, http.StatusOK, codersdk.AuditLogResponse{
		AuditLogs: convertAuditLogs(dblogs),
	})
}

func (api *API) auditLogCount(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if !api.Authorize(r, rbac.ActionRead, rbac.ResourceAuditLog) {
		httpapi.Forbidden(rw)
		return
	}

	count, err := api.Database.GetAuditLogCount(ctx)
	if err != nil {
		httpapi.InternalServerError(rw, err)
		return
	}

	httpapi.Write(rw, http.StatusOK, codersdk.AuditLogCountResponse{
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

	_, err = api.Database.InsertAuditLog(ctx, database.InsertAuditLogParams{
		ID:               uuid.New(),
		Time:             time.Now(),
		UserID:           user.ID,
		Ip:               ipNet,
		UserAgent:        r.UserAgent(),
		ResourceType:     database.ResourceTypeUser,
		ResourceID:       user.ID,
		ResourceTarget:   user.Username,
		Action:           database.AuditActionWrite,
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
	return fmt.Sprintf("{user} %s %s {target}",
		codersdk.AuditAction(alog.Action).FriendlyString(),
		codersdk.ResourceType(alog.ResourceType).FriendlyString(),
	)
}
