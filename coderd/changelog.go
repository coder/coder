package coderd

import (
	"io/fs"
	"net/http"
	"path"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/changelog"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/coderd/notifications"
	"github.com/coder/coder/v2/codersdk"
)

// listChangelogEntries lists the embedded changelog entries.
//
// @Summary List changelog entries
// @ID list-changelog-entries
// @Security CoderSessionToken
// @Produce json
// @Tags Changelog
// @Success 200 {object} codersdk.ListChangelogEntriesResponse
// @Router /changelog [get]
func (api *API) listChangelogEntries(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	entries, err := api.ChangelogStore.List()
	if err != nil {
		httpapi.InternalServerError(rw, err)
		return
	}

	resp := codersdk.ListChangelogEntriesResponse{
		Entries: make([]codersdk.ChangelogEntry, 0, len(entries)),
	}
	for _, e := range entries {
		imageURL := ""
		if e.Image != "" {
			imageURL = "/api/v2/changelog/assets/" + e.Image
		}

		resp.Entries = append(resp.Entries, codersdk.ChangelogEntry{
			Version:  e.Version,
			Title:    e.Title,
			Date:     e.Date,
			Summary:  e.Summary,
			ImageURL: imageURL,
		})
	}

	httpapi.Write(ctx, rw, http.StatusOK, resp)
}

// changelogEntryByVersion returns a single changelog entry by version.
//
// @Summary Get changelog entry
// @ID get-changelog-entry
// @Security CoderSessionToken
// @Produce json
// @Tags Changelog
// @Param version path string true "Version"
// @Success 200 {object} codersdk.ChangelogEntry
// @Router /changelog/{version} [get]
func (api *API) changelogEntryByVersion(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	version := chi.URLParam(r, "version")

	entry, err := api.ChangelogStore.Get(version)
	if err != nil {
		if _, listErr := api.ChangelogStore.List(); listErr != nil {
			httpapi.InternalServerError(rw, listErr)
			return
		}

		httpapi.Write(ctx, rw, http.StatusNotFound, codersdk.Response{
			Message: "Changelog entry not found.",
		})
		return
	}

	imageURL := ""
	if entry.Image != "" {
		imageURL = "/api/v2/changelog/assets/" + entry.Image
	}

	httpapi.Write(ctx, rw, http.StatusOK, codersdk.ChangelogEntry{
		Version:  entry.Version,
		Title:    entry.Title,
		Date:     entry.Date,
		Summary:  entry.Summary,
		ImageURL: imageURL,
		Content:  entry.Content,
	})
}

// changelogAsset serves embedded assets referenced by changelog entries.
//
// @Summary Get changelog asset
// @ID get-changelog-asset
// @Security CoderSessionToken
// @Produce octet-stream
// @Tags Changelog
// @Param path path string true "Asset path"
// @Success 200
// @Router /changelog/assets/{path} [get]
func (*API) changelogAsset(rw http.ResponseWriter, r *http.Request) {
	assetPath := chi.URLParam(r, "*")
	if !fs.ValidPath(assetPath) {
		http.NotFound(rw, r)
		return
	}

	data, err := fs.ReadFile(changelog.FS, path.Join("assets", assetPath))
	if err != nil {
		http.NotFound(rw, r)
		return
	}

	// Detect content type from the file extension.
	switch strings.ToLower(path.Ext(assetPath)) {
	case ".webp":
		rw.Header().Set("Content-Type", "image/webp")
	case ".png":
		rw.Header().Set("Content-Type", "image/png")
	case ".jpg", ".jpeg":
		rw.Header().Set("Content-Type", "image/jpeg")
	case ".svg":
		rw.Header().Set("Content-Type", "image/svg+xml")
	default:
		rw.Header().Set("Content-Type", "application/octet-stream")
	}

	rw.WriteHeader(http.StatusOK)
	_, _ = rw.Write(data)
}

// unreadChangelogNotification returns the most recent unread changelog inbox
// notification for the authenticated user.
//
// @Summary Get unread changelog notification
// @ID get-unread-changelog-notification
// @Security CoderSessionToken
// @Produce json
// @Tags Changelog
// @Success 200 {object} codersdk.UnreadChangelogNotificationResponse
// @Router /changelog/unread [get]
func (api *API) unreadChangelogNotification(rw http.ResponseWriter, r *http.Request) {
	var (
		ctx    = r.Context()
		apikey = httpmw.APIKey(r)
	)

	notifs, err := api.Database.GetFilteredInboxNotificationsByUserID(ctx, database.GetFilteredInboxNotificationsByUserIDParams{
		UserID:     apikey.UserID,
		Templates:  []uuid.UUID{notifications.TemplateChangelog},
		ReadStatus: database.InboxNotificationReadStatusUnread,
		LimitOpt:   1,
	})
	if err != nil {
		api.Logger.Error(ctx, "failed to get unread changelog notification", slog.Error(err))
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to get unread changelog notification.",
		})
		return
	}

	var notif *codersdk.InboxNotification
	if len(notifs) > 0 {
		converted := convertInboxNotificationResponse(ctx, api.Logger, notifs[0])
		notif = &converted
	}

	httpapi.Write(ctx, rw, http.StatusOK, codersdk.UnreadChangelogNotificationResponse{
		Notification: notif,
	})
}
