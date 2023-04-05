package coderd

import (
	"database/sql"
	"fmt"
	"net/http"
	"net/url"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/coderd/audit"
	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/coderd/httpapi"
	"github.com/coder/coder/codersdk"
)

// @Summary Create workspace proxy
// @ID create-workspace-proxy
// @Security CoderSessionToken
// @Accept json
// @Produce json
// @Tags Templates
// @Param request body codersdk.CreateWorkspaceProxyRequest true "Create workspace proxy request"
// @Success 201 {object} codersdk.WorkspaceProxy
// @Router /workspaceproxies [post]
func (api *API) postWorkspaceProxy(rw http.ResponseWriter, r *http.Request) {
	var (
		ctx               = r.Context()
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

	if err := validateProxyURL(req.URL); err != nil {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "URL is invalid.",
			Detail:  err.Error(),
		})
		return
	}

	if _, err := httpapi.CompileHostnamePattern(req.WildcardHostname); err != nil {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Wildcard URL is invalid.",
			Detail:  err.Error(),
		})
		return
	}

	proxy, err := api.Database.InsertWorkspaceProxy(ctx, database.InsertWorkspaceProxyParams{
		ID:               uuid.New(),
		Name:             req.Name,
		DisplayName:      req.DisplayName,
		Icon:             req.Icon,
		Url:              req.URL,
		WildcardHostname: req.WildcardHostname,
		CreatedAt:        database.Now(),
		UpdatedAt:        database.Now(),
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

// nolint:revive
func validateProxyURL(u string) error {
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
	return nil
}

// @Summary Get workspace proxies
// @ID get-workspace-proxies
// @Security CoderSessionToken
// @Produce json
// @Tags Enterprise
// @Success 200 {array} codersdk.WorkspaceProxy
// @Router /workspaceproxies [get]
func (api *API) workspaceProxies(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	proxies, err := api.Database.GetWorkspaceProxies(ctx)
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
		ID:               p.ID,
		Name:             p.Name,
		Icon:             p.Icon,
		URL:              p.Url,
		WildcardHostname: p.WildcardHostname,
		CreatedAt:        p.CreatedAt,
		UpdatedAt:        p.UpdatedAt,
		Deleted:          p.Deleted,
	}
}
