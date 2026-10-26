package coderd

import (
	"context"
	"mime"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/coderd/util/slice"
	"github.com/coder/coder/v2/coderd/x/chatfiles"
	"github.com/coder/coder/v2/coderd/x/coderkit"
	"github.com/coder/coder/v2/codersdk"
)

// artifactFrameCSP sandboxes an HTML artifact into an opaque origin, so it
// has no cookies, storage or same-origin API. Scripts and styles load only
// inline or from a few public CDNs, and the page has no other network.
const artifactFrameCSP = "sandbox allow-scripts; default-src 'none'; " +
	"script-src 'unsafe-inline' https://cdnjs.cloudflare.com https://unpkg.com https://cdn.tailwindcss.com https://code.jquery.com https://cdn.jsdelivr.net/npm/; " +
	"style-src 'unsafe-inline' https://cdnjs.cloudflare.com https://unpkg.com https://cdn.jsdelivr.net/npm/ https://fonts.googleapis.com; " +
	"font-src data: https://fonts.gstatic.com; img-src data: blob:; " +
	"connect-src 'none'; form-action 'none'; base-uri 'none'; frame-ancestors 'self'"

// listChatArtifacts lists the artifacts that the caller owns or can see
// through a chat project, across all organizations.
// EXPERIMENTAL
func (api *API) listChatArtifacts(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	apiKey := httpmw.APIKey(r)

	rows, err := api.Database.GetChatArtifactsOwnedOrInUserProjects(ctx, apiKey.UserID)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to list chat artifacts.",
			Detail:  err.Error(),
		})
		return
	}
	projects := api.newChatArtifactProjectVisibility(apiKey.UserID)
	artifacts := make([]codersdk.ChatArtifact, 0, len(rows))
	for _, row := range rows {
		showProject, err := projects.visible(ctx, row.ChatArtifact)
		if err != nil {
			httpapi.InternalServerError(rw, err)
			return
		}
		// Both queries select the same columns, so the rows convert.
		artifact := convertChatArtifact(database.GetChatArtifactByIDRow(row))
		if !showProject {
			hideChatArtifactProject(&artifact)
		}
		artifacts = append(artifacts, artifact)
	}
	httpapi.Write(ctx, rw, http.StatusOK, artifacts)
}

// getChatArtifact returns an artifact and the metadata of its versions.
// EXPERIMENTAL
//
//nolint:revive // get-return: revive assumes get* must be a getter, but this is an HTTP handler.
func (api *API) getChatArtifact(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	apiKey := httpmw.APIKey(r)
	row := httpmw.ChatArtifactParam(r)

	versions, err := api.Database.GetChatArtifactVersionsMetadata(ctx, row.ChatArtifact.ID)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to list chat artifact versions.",
			Detail:  err.Error(),
		})
		return
	}
	showProject, err := api.newChatArtifactProjectVisibility(apiKey.UserID).visible(ctx, row.ChatArtifact)
	if err != nil {
		httpapi.InternalServerError(rw, err)
		return
	}
	artifact := convertChatArtifact(row)
	if !showProject {
		hideChatArtifactProject(&artifact)
	}
	artifact.Versions = slice.List(versions, func(v database.GetChatArtifactVersionsMetadataRow) codersdk.ChatArtifactVersion {
		return convertChatArtifactVersion(v, apiKey.UserID)
	})
	httpapi.Write(ctx, rw, http.StatusOK, artifact)
}

// getChatArtifactVersion serves the bytes of one version with the headers
// of chat file downloads. HTML and SVG download as attachments.
// EXPERIMENTAL
//
//nolint:revive // get-return: revive assumes get* must be a getter, but this is an HTTP handler.
func (api *API) getChatArtifactVersion(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	version, ok := api.chatArtifactVersionFromURL(rw, r)
	if !ok {
		return
	}
	rw.Header().Set("Cache-Control", "private, no-store")
	rw.Header().Set("X-Content-Type-Options", "nosniff")
	rw.Header().Set("Content-Type", version.Mimetype)
	// HTML is stored as text/plain, so the name decides too: HTML only
	// renders through the sandboxed frame route.
	disposition := "attachment"
	if chatfiles.IsInlineRenderableStoredMediaType(version.Mimetype) && !isHTMLName(version.Name) {
		disposition = "inline"
	}
	rw.Header().Set("Content-Disposition", mime.FormatMediaType(disposition, map[string]string{"filename": version.Name}))
	data := version.Data
	if isHTMLName(version.Name) {
		// A download must look like the frame, so it gets the kit inline.
		data = coderkit.Pack(data, coderkit.ThemeNone)
	}
	rw.Header().Set("Content-Length", strconv.Itoa(len(data)))
	rw.WriteHeader(http.StatusOK)
	if _, err := rw.Write(data); err != nil {
		api.Logger.Debug(ctx, "failed to write chat artifact version", slog.Error(err))
	}
}

// chatArtifactFrame serves an HTML version for an iframe with a CSP
// sandbox. Sec-Fetch-Dest stops top-level visits only; the sandbox is the
// boundary. A page that links coder-kit.css gets the Coder Kit inline, in
// the theme that the theme query parameter names.
// EXPERIMENTAL
func (api *API) chatArtifactFrame(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if r.Header.Get("Sec-Fetch-Dest") != "iframe" {
		httpapi.ResourceNotFound(rw)
		return
	}
	version, ok := api.chatArtifactVersionFromURL(rw, r)
	if !ok {
		return
	}
	if !isHTMLName(version.Name) {
		httpapi.ResourceNotFound(rw)
		return
	}
	rw.Header().Set("Content-Type", "text/html; charset=utf-8")
	rw.Header().Set("Content-Security-Policy", artifactFrameCSP)
	rw.Header().Set("Cache-Control", "private, no-store")
	rw.Header().Set("Referrer-Policy", "no-referrer")
	rw.Header().Set("X-Content-Type-Options", "nosniff")
	rw.WriteHeader(http.StatusOK)
	data := coderkit.Pack(version.Data, coderkit.ParseTheme(r.URL.Query().Get("theme")))
	if _, err := rw.Write(data); err != nil {
		api.Logger.Debug(ctx, "failed to write chat artifact frame", slog.Error(err))
	}
}

// chatArtifactVersionFromURL loads the version named by the URL. It writes
// 404 for a bad or missing version.
func (api *API) chatArtifactVersionFromURL(rw http.ResponseWriter, r *http.Request) (database.ChatArtifactVersion, bool) {
	ctx := r.Context()
	artifact := httpmw.ChatArtifactParam(r)
	number, err := strconv.ParseInt(chi.URLParam(r, "version"), 10, 32)
	if err != nil || number <= 0 {
		httpapi.ResourceNotFound(rw)
		return database.ChatArtifactVersion{}, false
	}
	version, err := api.Database.GetChatArtifactVersion(ctx, database.GetChatArtifactVersionParams{
		ArtifactID: artifact.ChatArtifact.ID,
		Version:    int32(number),
	})
	if httpapi.Is404Error(err) {
		httpapi.ResourceNotFound(rw)
		return database.ChatArtifactVersion{}, false
	}
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to get chat artifact version.",
			Detail:  err.Error(),
		})
		return database.ChatArtifactVersion{}, false
	}
	return version, true
}

func isHTMLName(name string) bool {
	name = strings.ToLower(name)
	return strings.HasSuffix(name, ".html") || strings.HasSuffix(name, ".htm")
}

// chatArtifactProjectVisibility decides whether a caller sees an
// artifact's project. A direct sharee who cannot read the project must not
// learn its ID or name. Results are cached per project for one request.
type chatArtifactProjectVisibility struct {
	api      *API
	callerID uuid.UUID
	readable map[uuid.UUID]bool
}

func (api *API) newChatArtifactProjectVisibility(callerID uuid.UUID) *chatArtifactProjectVisibility {
	return &chatArtifactProjectVisibility{api: api, callerID: callerID, readable: map[uuid.UUID]bool{}}
}

// visible reports whether the caller may see the artifact's project: the
// artifact owner may, and so may anyone who can read the project.
func (v *chatArtifactProjectVisibility) visible(ctx context.Context, artifact database.ChatArtifact) (bool, error) {
	if !artifact.ProjectID.Valid {
		return false, nil
	}
	if artifact.OwnerID == v.callerID {
		return true, nil
	}
	projectID := artifact.ProjectID.UUID
	if readable, ok := v.readable[projectID]; ok {
		return readable, nil
	}
	// The lookup runs as the caller, so dbauthz answers the read check.
	_, err := v.api.Database.GetChatProjectByID(ctx, projectID)
	if err != nil && !httpapi.Is404Error(err) {
		return false, xerrors.Errorf("get chat artifact project: %w", err)
	}
	v.readable[projectID] = err == nil
	return err == nil, nil
}

// hideChatArtifactProject removes the project fields for a caller who
// cannot read the project.
func hideChatArtifactProject(artifact *codersdk.ChatArtifact) {
	artifact.ProjectID = nil
	artifact.ProjectName = ""
}

// convertChatArtifact sets every field. Callers hide the project with
// hideChatArtifactProject when chatArtifactProjectVisibility says so.
func convertChatArtifact(row database.GetChatArtifactByIDRow) codersdk.ChatArtifact {
	a := row.ChatArtifact
	out := codersdk.ChatArtifact{
		ID:              a.ID,
		OrganizationID:  a.OrganizationID,
		OwnerID:         a.OwnerID,
		OwnerUsername:   row.OwnerUsername,
		Title:           a.Title,
		LatestVersion:   row.LatestVersion,
		LatestName:      row.LatestName,
		LatestMediaType: row.LatestMediaType,
		CreatedAt:       a.CreatedAt,
		UpdatedAt:       a.UpdatedAt,
	}
	if a.ProjectID.Valid {
		out.ProjectID = &a.ProjectID.UUID
		out.ProjectName = row.ProjectName
	}
	return out
}

// convertChatArtifactVersion names the publishing chat only to its owner,
// because other viewers cannot open that chat.
func convertChatArtifactVersion(v database.GetChatArtifactVersionsMetadataRow, callerID uuid.UUID) codersdk.ChatArtifactVersion {
	out := codersdk.ChatArtifactVersion{
		Version:           v.Version,
		Label:             v.Label,
		Name:              v.Name,
		MediaType:         v.Mimetype,
		Size:              v.Size,
		CreatedByUsername: v.CreatedByUsername,
		CreatedAt:         v.CreatedAt,
	}
	if v.CreatedBy.Valid {
		out.CreatedBy = &v.CreatedBy.UUID
	}
	if v.ChatID.Valid {
		out.ChatID = &v.ChatID.UUID
		if v.ChatOwnerID.Valid && v.ChatOwnerID.UUID == callerID {
			out.ChatTitle = v.ChatTitle
		}
	}
	return out
}
