package agentfiles

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"

	"github.com/google/uuid"
	"github.com/spf13/afero"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/x/chatfiles"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
)

// maxUploadChatFileCollisionAttempts bounds the collision-suffix
// search so a poisoned directory cannot wedge the handler.
const maxUploadChatFileCollisionAttempts = 1000

// HandleUploadChatFile streams a request body into the workspace upload directory.
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

	// Workspace uploads intentionally stream without a MaxBytesReader cap.
	// The uploaded bytes land in the user's workspace filesystem rather than
	// coderd storage, so workspace disk limits are the enforcement boundary.
	dir := chatfiles.WorkspaceUploadDir(home, chatID.String())
	finalName, finalPath, size, err := api.writeUploadExclusiveSecure(home, chatID.String(), dir, name, r.Body)
	if errors.Is(err, errUploadSecureUnsupported) {
		err = api.prepareUploadDirPath(home, chatID.String(), dir)
		if err == nil {
			finalName, finalPath, size, err = api.writeUploadExclusivePath(dir, name, r.Body)
		}
	}
	if err != nil {
		status := http.StatusInternalServerError
		switch {
		case errors.Is(err, errUploadCollisionExhausted):
			status = http.StatusConflict
		case errors.Is(err, errUploadDirSymlink), errors.Is(err, os.ErrPermission):
			status = http.StatusForbidden
		}
		httpapi.Write(ctx, rw, status, codersdk.Response{
			Message: err.Error(),
		})
		return
	}

	httpapi.Write(ctx, rw, http.StatusOK, workspacesdk.AgentUploadChatFileResponse{
		Path: finalPath,
		Name: finalName,
		Size: size,
	})
}

var errUploadDirSymlink = xerrors.New("workspace upload directory must not contain symlinks")

var errUploadSecureUnsupported = xerrors.New("secure workspace upload unsupported by filesystem")

var errUploadCollisionExhausted = xerrors.New("too many existing files with the same name")

func (api *API) prepareUploadDirPath(homeDir, chatID, dir string) error {
	if err := api.rejectSymlinkedUploadDir(homeDir, chatID); err != nil {
		return err
	}
	if err := api.filesystem.MkdirAll(dir, 0o755); err != nil {
		return xerrors.Errorf("create upload directory: %w", err)
	}
	return api.rejectSymlinkedUploadDir(homeDir, chatID)
}

func (api *API) rejectSymlinkedUploadDir(homeDir, chatID string) error {
	lstater, ok := api.filesystem.(afero.Lstater)
	if !ok {
		return nil
	}
	for _, dir := range []string{
		filepath.Join(homeDir, ".coder"),
		filepath.Join(homeDir, chatfiles.WorkspaceChatsDir),
		chatfiles.WorkspaceChatDir(homeDir, chatID),
		chatfiles.WorkspaceUploadDir(homeDir, chatID),
	} {
		info, _, err := lstater.LstatIfPossible(dir)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return xerrors.Errorf("check upload directory: %w", err)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return xerrors.Errorf("%w: %s", errUploadDirSymlink, dir)
		}
	}
	return nil
}

func (api *API) writeUploadExclusivePath(dir, name string, r io.Reader) (finalName, finalPath string, size int64, err error) {
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
