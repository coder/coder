package coderd

import (
	"mime"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/coderd/util/slice"
	"github.com/coder/coder/v2/coderd/x/chatfiles"
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
	httpapi.Write(ctx, rw, http.StatusOK, slice.List(rows, func(row database.GetChatArtifactsOwnedOrInUserProjectsRow) codersdk.ChatArtifact {
		// Both queries select the same columns, so the rows convert.
		return convertChatArtifact(database.GetChatArtifactByIDRow(row))
	}))
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
	artifact := convertChatArtifact(row)
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
	rw.Header().Set("Content-Length", strconv.Itoa(len(version.Data)))
	rw.WriteHeader(http.StatusOK)
	if _, err := rw.Write(version.Data); err != nil {
		api.Logger.Debug(ctx, "failed to write chat artifact version", slog.Error(err))
	}
}

// chatArtifactFrame serves an HTML version for an iframe with a CSP
// sandbox. Sec-Fetch-Dest stops top-level visits only; the sandbox is the
// boundary.
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
	if _, err := rw.Write(version.Data); err != nil {
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
