package agentfiles

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/x/chatfiles"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
)

// maxUploadChatFileCollisionAttempts bounds the collision-suffix
// search so a poisoned directory cannot wedge the handler.
const maxUploadChatFileCollisionAttempts = 1000

// HandleUploadChatFile streams a request body into the agent's home
// directory under .coder/chats/<chat-id>/files/<sanitized-name>.
// Concurrent uploads of the same name are resolved with collision
// suffixes via atomic O_EXCL creation. On any write error the
// partial file is removed so callers never observe a half-written
// upload.
//
// Query parameters: chat_id (required, the chat UUID; used as the
// per-chat subdirectory), name (required, original filename).
func (api *API) HandleUploadChatFile(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	query := r.URL.Query()
	parser := httpapi.NewQueryParamParser().
		RequiredNotEmpty("chat_id").
		RequiredNotEmpty("name")
	chatID := parser.UUID(query, uuid.Nil, "chat_id")
	rawName := parser.String(query, "", "name")
	parser.ErrorExcessParams(query)
	if len(parser.Errors) > 0 {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message:     "Query parameters have invalid values.",
			Validations: parser.Errors,
		})
		return
	}

	name, err := chatfiles.SanitizeWorkspaceUploadName(rawName)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: err.Error(),
		})
		return
	}

	home, err := os.UserHomeDir()
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: fmt.Sprintf("resolve home directory: %s", err),
		})
		return
	}

	dir := chatfiles.WorkspaceUploadDir(home, chatID.String())
	if err := api.filesystem.MkdirAll(dir, 0o755); err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, os.ErrPermission) {
			status = http.StatusForbidden
		}
		httpapi.Write(ctx, rw, status, codersdk.Response{
			Message: fmt.Sprintf("create upload directory: %s", err),
		})
		return
	}

	finalName, finalPath, size, err := api.writeUploadExclusive(dir, name, r.Body)
	if err != nil {
		status := http.StatusInternalServerError
		switch {
		case errors.Is(err, errUploadCollisionExhausted):
			status = http.StatusConflict
		case errors.Is(err, os.ErrPermission):
			status = http.StatusForbidden
		}
		httpapi.Write(ctx, rw, status, codersdk.Response{
			Message: err.Error(),
		})
		return
	}

	httpapi.Write(ctx, rw, http.StatusOK, workspacesdk.UploadChatFileResponse{
		Path: finalPath,
		Name: finalName,
		Size: size,
	})
}

// HandleDeleteChatFiles removes the chat-scoped workspace upload directory.
// It is idempotent so coderd can call it as best-effort lifecycle cleanup.
//
// Query parameters: chat_id (required, the chat UUID; used as the
// per-chat subdirectory).
func (api *API) HandleDeleteChatFiles(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	query := r.URL.Query()
	parser := httpapi.NewQueryParamParser().
		RequiredNotEmpty("chat_id")
	chatID := parser.UUID(query, uuid.Nil, "chat_id")
	parser.ErrorExcessParams(query)
	if len(parser.Errors) > 0 {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message:     "Query parameters have invalid values.",
			Validations: parser.Errors,
		})
		return
	}

	home, err := os.UserHomeDir()
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: fmt.Sprintf("resolve home directory: %s", err),
		})
		return
	}

	dir := chatfiles.WorkspaceChatDir(home, chatID.String())
	if err := api.filesystem.RemoveAll(dir); err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, os.ErrPermission) {
			status = http.StatusForbidden
		}
		httpapi.Write(ctx, rw, status, codersdk.Response{
			Message: fmt.Sprintf("delete chat upload directory: %s", err),
		})
		return
	}

	httpapi.Write(ctx, rw, http.StatusOK, codersdk.Response{
		Message: "Chat workspace files deleted.",
	})
}

// errUploadCollisionExhausted is returned when the collision-suffix
// search exceeds maxUploadChatFileCollisionAttempts.
var errUploadCollisionExhausted = xerrors.New("too many existing files with the same name")

// writeUploadExclusive reserves a target path before copying so concurrent
// same-name uploads cannot overwrite each other.
func (api *API) writeUploadExclusive(dir, name string, r io.Reader) (finalName, finalPath string, size int64, err error) {
	for i := 1; i <= maxUploadChatFileCollisionAttempts; i++ {
		candidate := chatfiles.AddCollisionSuffix(name, i)
		candidatePath := filepath.Join(dir, candidate)

		f, err := api.filesystem.OpenFile(candidatePath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
		if err != nil {
			if errors.Is(err, os.ErrExist) {
				continue
			}
			return "", "", 0, xerrors.Errorf("create upload target: %w", err)
		}

		n, err := io.Copy(f, r)
		if cerr := f.Close(); err == nil {
			err = cerr
		}
		if err != nil {
			_ = api.filesystem.Remove(candidatePath)
			return "", "", 0, xerrors.Errorf("write upload: %w", err)
		}
		return candidate, candidatePath, n, nil
	}
	return "", "", 0, errUploadCollisionExhausted
}
