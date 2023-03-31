package coderd

import (
	"database/sql"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"golang.org/x/xerrors"

	"github.com/coder/coder/coderd/audit"
	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/coderd/httpapi"
	"github.com/coder/coder/coderd/httpmw"
	"github.com/coder/coder/codersdk"
	"github.com/google/uuid"
)

// @Summary Create workspace proxy for organization
// @ID create-workspace-proxy-for-organization
// @Security CoderSessionToken
// @Accept json
// @Produce json
// @Tags Templates
// @Param request body codersdk.CreateWorkspaceProxyRequest true "Create workspace proxy request"
// @Param organization path string true "Organization ID"
// @Success 201 {object} codersdk.WorkspaceProxy
// @Router /organizations/{organization}/workspaceproxies [post]
func (api *API) postWorkspaceProxyByOrganization(rw http.ResponseWriter, r *http.Request) {
	var (
		ctx               = r.Context()
		org               = httpmw.OrganizationParam(r)
		auditor           = api.AGPL.Auditor.Load()
		aReq, commitAudit = audit.InitRequest[database.WorkspaceProxy](rw, &audit.RequestParams{
			Audit:   *auditor,
			Log:     api.Logger,
			Request: r,
			Action:  database.AuditActionCreate,
		})
	)
	defer commitAudit()

	var req codersdk.CreateWorkspaceProxyRequest
	if !httpapi.Read(ctx, rw, r, &req) {
		return
	}

	if err := validateProxyURL(req.URL, false); err != nil {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "URL is invalid.",
			Detail:  err.Error(),
		})
		return
	}

	if err := validateProxyURL(req.WildcardURL, true); err != nil {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Wildcard URL is invalid.",
			Detail:  err.Error(),
		})
		return
	}

	proxy, err := api.Database.InsertWorkspaceProxy(ctx, database.InsertWorkspaceProxyParams{
		ID:             uuid.New(),
		OrganizationID: org.ID,
		Name:           req.Name,
		DisplayName:    req.DisplayName,
		Icon:           req.Icon,
		// TODO: validate URLs
		Url:         req.URL,
		WildcardUrl: req.WildcardURL,
		CreatedAt:   database.Now(),
		UpdatedAt:   database.Now(),
	})
	if database.IsUniqueViolation(err) {
		httpapi.Write(ctx, rw, http.StatusConflict, codersdk.Response{
			Message: fmt.Sprintf("Workspace proxy with name %q already exists.", req.Name),
		})
		return
	}
	if err != nil {
		httpapi.InternalServerError(rw, err)
		return
	}

	aReq.New = proxy
	httpapi.Write(ctx, rw, http.StatusCreated, convertProxy(proxy))
}

func validateProxyURL(u string, wildcard bool) error {
	p, err := url.Parse(u)
	if err != nil {
		return err
	}
	if p.Scheme != "http" && p.Scheme != "https" {
		return xerrors.New("scheme must be http or https")
	}
	if !(p.Path == "/" || p.Path == "") {
		return xerrors.New("path must be empty or /")
	}
	if wildcard {
		if !strings.HasPrefix(p.Host, "*.") {
			return xerrors.Errorf("wildcard URL must have a wildcard subdomain (e.g. *.example.com)")
		}
	}
	return nil
}

// @Summary Get workspace proxies
// @ID get-workspace-proxies
// @Security CoderSessionToken
// @Produce json
// @Tags Enterprise
// @Param organization path string true "Organization ID" format(uuid)
// @Success 200 {array} codersdk.WorkspaceProxy
// @Router /organizations/{organization}/workspaceproxies [get]
func (api *API) workspaceProxiesByOrganization(rw http.ResponseWriter, r *http.Request) {
	var (
		ctx = r.Context()
		org = httpmw.OrganizationParam(r)
	)

	proxies, err := api.Database.GetWorkspaceProxies(ctx, org.ID)
	if err != nil && !xerrors.Is(err, sql.ErrNoRows) {
		httpapi.InternalServerError(rw, err)
		return
	}

	httpapi.Write(ctx, rw, http.StatusOK, convertProxies(proxies))
}

func convertProxies(p []database.WorkspaceProxy) []codersdk.WorkspaceProxy {
	resp := make([]codersdk.WorkspaceProxy, 0, len(p))
	for _, proxy := range p {
		resp = append(resp, convertProxy(proxy))
	}
	return resp
}

func convertProxy(p database.WorkspaceProxy) codersdk.WorkspaceProxy {
	return codersdk.WorkspaceProxy{
		ID:             p.ID,
		OrganizationID: p.OrganizationID,
		Name:           p.Name,
		Icon:           p.Icon,
		Url:            p.Url,
		WildcardUrl:    p.WildcardUrl,
		CreatedAt:      p.CreatedAt,
		UpdatedAt:      p.UpdatedAt,
		Deleted:        p.Deleted,
	}
}
