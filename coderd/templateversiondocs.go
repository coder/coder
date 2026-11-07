package coderd

import (
	"archive/tar"
	"bytes"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/codersdk"
)

// @Summary Get template version documentation
// @ID get-template-version-documentation
// @Security CoderSessionToken
// @Produce json
// @Tags Templates
// @Param templateversion path string true "Template version ID" format(uuid)
// @Success 200 {object} codersdk.TemplateVersionDocumentation
// @Router /api/v2/templateversions/{templateversion}/docs [get]
func (api *API) templateVersionDocumentation(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	version := httpmw.TemplateVersionParam(r)
	job, err := api.Database.GetProvisionerJobByID(ctx, version.JobID)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{Message: "Error fetching template job.", Detail: err.Error()})
		return
	}
	// The middleware has authorized template read access. Only root Markdown
	// documents are exposed; the source archive remains restricted.
	// nolint:gocritic // Template readers need documentation without source access.
	file, err := api.Database.GetFileByID(dbauthz.AsSystemRestricted(ctx), job.FileID)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{Message: "Error fetching template documentation.", Detail: err.Error()})
		return
	}
	docs := codersdk.TemplateVersionDocumentation{Files: map[string]string{}}
	reader := tar.NewReader(bytes.NewReader(file.Data))
	for {
		header, err := reader.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{Message: "Error reading template documentation.", Detail: err.Error()})
			return
		}
		name := strings.TrimPrefix(header.Name, "./")
		if header.Typeflag != tar.TypeReg || strings.ContainsAny(name, "/\\") || !strings.HasSuffix(strings.ToLower(name), ".md") {
			continue
		}
		if header.Size > HTTPFileMaxBytes {
			httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{Message: "Template document exceeds the file size limit."})
			return
		}
		content, err := io.ReadAll(reader)
		if err != nil {
			httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{Message: "Error reading template document.", Detail: err.Error()})
			return
		}
		docs.Files[name] = string(content)
	}
	httpapi.Write(ctx, rw, http.StatusOK, docs)
}
