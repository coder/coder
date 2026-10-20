package agentfiles

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/spf13/afero"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/x/chatfiles"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
	"github.com/coder/coder/v2/cryptorand"
)

// maxUploadChatFileCollisionAttempts bounds the collision-suffix
// search so a poisoned directory cannot wedge the handler.
const maxUploadChatFileCollisionAttempts = 1000

// uploadChatFileIdleTimeout bounds how long an upload may stall between
// chunks. The agent HTTP server's ReadTimeout and WriteTimeout are fixed
// deadlines measured from the start of the request, which would cut off
// any upload slower than that even though this endpoint has no size cap,
// so the body reader pushes both deadlines forward while bytes arrive.
const uploadChatFileIdleTimeout = 20 * time.Second

// uploadTempMarker identifies in-progress upload files. Sanitized upload
// names never start with a dot, so ".<name>.upload-<random>" cannot
// collide with a published upload.
const uploadTempMarker = ".upload-"

// staleUploadTempAge is how old an upload temp file must be before a
// later upload sweeps it. Active uploads rewrite their temp file at
// least once per uploadChatFileIdleTimeout, so older files are abandoned.
const staleUploadTempAge = time.Hour

// deadlineExtendingReader extends the request's read and write deadlines
// every time a chunk of the body arrives, turning the server's fixed
// per-request timeouts into an idle timeout for streamed uploads.
type deadlineExtendingReader struct {
	ctx      context.Context
	logger   slog.Logger
	r        io.Reader
	rc       *http.ResponseController
	received int64
	warned   bool
}

func (d *deadlineExtendingReader) extend() {
	deadline := time.Now().Add(uploadChatFileIdleTimeout)
	err := d.rc.SetReadDeadline(deadline)
	_ = d.rc.SetWriteDeadline(deadline)
	// ErrNotSupported means a ResponseWriter wrapper hides the connection,
	// so the server's fixed timeouts will still cut off slow uploads.
	if errors.Is(err, http.ErrNotSupported) && !d.warned {
		d.warned = true
		d.logger.Warn(d.ctx, "cannot extend workspace chat file upload deadlines", slog.Error(err))
	}
}

func (d *deadlineExtendingReader) Read(p []byte) (int, error) {
	n, err := d.r.Read(p)
	if n > 0 {
		d.received += int64(n)
		d.extend()
	}
	return n, err
}

// HandleUploadChatFile streams a request body into the workspace upload directory.
//
// This endpoint is not subject to CODER_AGENT_BLOCK_FILE_TRANSFER. That flag
// guards SSH-level transfer tools (scp, rsync, ftp, nc); chat uploads are an
// agent API capability like write-file and edit-files, which already operate
// outside the SSH transfer path.
func (api *API) HandleUploadChatFile(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	start := time.Now()

	query := r.URL.Query()
	parser := httpapi.NewQueryParamParser().
		RequiredNotEmpty("chat_id").
		RequiredNotEmpty("name")
	chatID := parser.UUID(query, uuid.Nil, "chat_id")
	rawName := parser.String(query, "", "name")
	fileID := parser.UUID(query, uuid.Nil, "file_id")
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

	home, err := api.envInfo.HomeDir()
	if err != nil {
		api.logger.Warn(ctx, "workspace chat file upload failed",
			slog.F("chat_id", chatID),
			slog.F("name", name),
			slog.F("status", http.StatusInternalServerError),
			slog.Error(err),
		)
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: fmt.Sprintf("resolve home directory: %s", err),
		})
		return
	}

	body := &deadlineExtendingReader{ctx: ctx, logger: api.logger, r: r.Body, rc: http.NewResponseController(rw)}
	body.extend()

	// Workspace uploads intentionally stream without a MaxBytesReader cap.
	// The uploaded bytes land in the user's workspace filesystem rather than
	// coderd storage, so workspace disk limits are the enforcement boundary.
	target := uploadTarget{
		homeDir: home,
		chatID:  chatID.String(),
		dir:     chatfiles.WorkspaceUploadDir(home, chatID.String()),
	}
	if fileID != uuid.Nil {
		target.fileID = fileID.String()
		target.dir = chatfiles.WorkspaceUploadFileDir(home, target.chatID, target.fileID)
	}
	finalName, finalPath, size, err := api.writeUploadExclusiveSecure(target, name, body)
	if errors.Is(err, errUploadSecureUnsupported) {
		err = api.prepareUploadDirPath(target)
		if err == nil {
			finalName, finalPath, size, err = api.writeUploadExclusivePath(target.dir, name, target.mode(), body)
		}
	}
	if err != nil {
		status := http.StatusInternalServerError
		switch {
		case errors.Is(err, errUploadCollisionExhausted), errors.Is(err, errUploadTargetNotRegular):
			status = http.StatusConflict
		case errors.Is(err, errUploadDirSymlink), errors.Is(err, os.ErrPermission):
			status = http.StatusForbidden
		case errors.Is(err, errUploadPublishInProgress):
			status = http.StatusServiceUnavailable
		}
		api.logger.Warn(ctx, "workspace chat file upload failed",
			slog.F("chat_id", chatID),
			slog.F("file_id", fileID),
			slog.F("name", name),
			slog.F("bytes_received", body.received),
			slog.F("duration", time.Since(start)),
			slog.F("status", status),
			slog.Error(err),
		)
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

var errUploadTargetNotRegular = xerrors.New("workspace upload target exists and is not a regular file")

// errUploadPublishInProgress means a write-once upload found the empty
// placeholder of a concurrent upload of the same file that has not
// renamed its content into place yet.
var errUploadPublishInProgress = xerrors.New("another upload of this file is still being published")

// checkWriteOnceExisting rejects an existing empty file when this upload
// has content, since the fallback writers publish through an empty
// placeholder and acknowledging it could report a file that never gets
// its content.
func checkWriteOnceExisting(existingSize, uploadedSize int64) error {
	if existingSize == 0 && uploadedSize > 0 {
		return errUploadPublishInProgress
	}
	return nil
}

// uploadPublishMode selects how an upload claims its final name.
type uploadPublishMode int

const (
	// uploadPublishUniqueName adds a collision suffix until the name is free.
	uploadPublishUniqueName uploadPublishMode = iota
	// uploadPublishWriteOnce publishes only under the exact name and treats
	// an existing regular file there as the result of an earlier identical
	// upload, so retried deliveries of one chat file are idempotent.
	uploadPublishWriteOnce
)

// uploadTarget is the upload directory below the agent home directory.
type uploadTarget struct {
	homeDir string
	chatID  string
	// fileID is empty unless the upload is write-once, in which case dir
	// is the per-file directory below the chat's files directory.
	fileID string
	dir    string
}

func (t uploadTarget) mode() uploadPublishMode {
	if t.fileID != "" {
		return uploadPublishWriteOnce
	}
	return uploadPublishUniqueName
}

func (api *API) prepareUploadDirPath(target uploadTarget) error {
	if err := api.rejectSymlinkedUploadDir(target); err != nil {
		return err
	}
	if err := api.filesystem.MkdirAll(target.dir, 0o755); err != nil {
		return xerrors.Errorf("create upload directory: %w", err)
	}
	return api.rejectSymlinkedUploadDir(target)
}

func (api *API) rejectSymlinkedUploadDir(target uploadTarget) error {
	lstater, ok := api.filesystem.(afero.Lstater)
	if !ok {
		return nil
	}
	dirs := []string{
		filepath.Join(target.homeDir, ".coder"),
		filepath.Join(target.homeDir, chatfiles.WorkspaceChatsDir),
		chatfiles.WorkspaceChatDir(target.homeDir, target.chatID),
		chatfiles.WorkspaceUploadDir(target.homeDir, target.chatID),
	}
	if target.fileID != "" {
		dirs = append(dirs, chatfiles.WorkspaceUploadFileDir(target.homeDir, target.chatID, target.fileID))
	}
	for _, dir := range dirs {
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

func (api *API) writeUploadExclusivePath(dir, name string, mode uploadPublishMode, r io.Reader) (finalName, finalPath string, size int64, err error) {
	api.sweepStaleUploadTempsPath(dir)

	tmp, err := uploadTempName(name)
	if err != nil {
		return "", "", 0, err
	}
	tmpPath := filepath.Join(dir, tmp)
	f, err := api.filesystem.OpenFile(tmpPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return "", "", 0, xerrors.Errorf("create upload temp file: %w", err)
	}
	defer func() { _ = api.filesystem.Remove(tmpPath) }()

	n, err := io.Copy(f, r)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return "", "", 0, xerrors.Errorf("write upload: %w", err)
	}

	for i := 1; i <= maxUploadChatFileCollisionAttempts; i++ {
		candidate := chatfiles.AddCollisionSuffix(name, i)
		candidatePath := filepath.Join(dir, candidate)

		// afero has no hard links, so reserve the name with an exclusive
		// empty placeholder and rename the finished temp file over it.
		placeholder, err := api.filesystem.OpenFile(candidatePath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
		if err != nil {
			if errors.Is(err, os.ErrExist) {
				if mode == uploadPublishWriteOnce {
					existingSize, err := api.existingUploadSizePath(candidatePath)
					if err != nil {
						return "", "", 0, err
					}
					if err := checkWriteOnceExisting(existingSize, n); err != nil {
						return "", "", 0, err
					}
					return candidate, candidatePath, existingSize, nil
				}
				continue
			}
			return "", "", 0, xerrors.Errorf("create upload target: %w", err)
		}
		err = placeholder.Close()
		if err == nil {
			err = api.filesystem.Rename(tmpPath, candidatePath)
		}
		if err != nil {
			_ = api.filesystem.Remove(candidatePath)
			return "", "", 0, xerrors.Errorf("publish upload: %w", err)
		}
		return candidate, candidatePath, n, nil
	}
	return "", "", 0, errUploadCollisionExhausted
}

// existingUploadSizePath returns the size of the regular file a write-once
// upload found at its target path.
func (api *API) existingUploadSizePath(p string) (int64, error) {
	var info os.FileInfo
	var err error
	if lstater, ok := api.filesystem.(afero.Lstater); ok {
		info, _, err = lstater.LstatIfPossible(p)
	} else {
		info, err = api.filesystem.Stat(p)
	}
	if err != nil {
		return 0, xerrors.Errorf("check existing upload: %w", err)
	}
	if !info.Mode().IsRegular() {
		return 0, xerrors.Errorf("%w: %s", errUploadTargetNotRegular, p)
	}
	return info.Size(), nil
}

func (api *API) sweepStaleUploadTempsPath(dir string) {
	infos, err := afero.ReadDir(api.filesystem, dir)
	if err != nil {
		return
	}
	for _, info := range infos {
		if isUploadTempName(info.Name()) && isStaleUploadTemp(info) {
			_ = api.filesystem.Remove(filepath.Join(dir, info.Name()))
		}
	}
}

// uploadTempName returns a unique hidden name for streaming an upload.
// The upload name is shortened so the result stays within NAME_MAX.
func uploadTempName(name string) (string, error) {
	random, err := cryptorand.HexString(16)
	if err != nil {
		return "", xerrors.Errorf("generate upload temp name: %w", err)
	}
	maxNameBytes := chatfiles.MaxWorkspaceUploadFileNameBytes - len(".") - len(uploadTempMarker) - len(random)
	if len(name) > maxNameBytes {
		name = strings.ToValidUTF8(name[:maxNameBytes], "")
	}
	return "." + name + uploadTempMarker + random, nil
}

func isUploadTempName(name string) bool {
	return strings.HasPrefix(name, ".") && strings.Contains(name, uploadTempMarker)
}

func isStaleUploadTemp(info os.FileInfo) bool {
	return info.Mode().IsRegular() && time.Since(info.ModTime()) > staleUploadTempAge
}
