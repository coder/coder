package chatd

import (
	"context"
	"fmt"
	"io"

	"github.com/google/uuid"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
)

// stopAndStoreRecording stops the desktop recording, downloads the
// MP4, and stores it in chat_files. Only called when the subagent
// completed successfully. Returns the file ID on success, empty
// string on any failure. All errors are logged but not propagated
// — recording is best-effort.
func (p *Server) stopAndStoreRecording(
	ctx context.Context,
	conn workspacesdk.AgentConn,
	recordingID string,
	ownerID uuid.UUID,
	workspaceID uuid.NullUUID,
) string {
	select {
	case p.recordingSem <- struct{}{}:
		defer func() { <-p.recordingSem }()
	case <-ctx.Done():
		p.logger.Warn(ctx, "context canceled waiting for recording semaphore", slog.Error(ctx.Err()))
		return ""
	}

	body, err := conn.StopDesktopRecording(ctx,
		workspacesdk.StopDesktopRecordingRequest{RecordingID: recordingID})
	if err != nil {
		p.logger.Warn(ctx, "failed to stop desktop recording",
			slog.Error(err))
		return ""
	}
	type readResult struct {
		data []byte
		err  error
	}
	ch := make(chan readResult, 1)
	go func() {
		data, err := io.ReadAll(io.LimitReader(body, workspacesdk.MaxRecordingSize+1))
		ch <- readResult{data, err}
	}()

	var data []byte
	select {
	case res := <-ch:
		body.Close()
		data = res.data
		if res.err != nil {
			p.logger.Warn(ctx, "failed to read recording data", slog.Error(res.err))
			return ""
		}
	case <-ctx.Done():
		body.Close()
		p.logger.Warn(ctx, "context canceled while reading recording data", slog.Error(ctx.Err()))
		return ""
	}
	if len(data) > workspacesdk.MaxRecordingSize {
		p.logger.Warn(ctx, "recording data exceeds maximum size, skipping store",
			slog.F("size", len(data)),
			slog.F("max_size", workspacesdk.MaxRecordingSize))
		return ""
	}
	if len(data) == 0 {
		p.logger.Warn(ctx, "recording data is empty, skipping store")
		return ""
	}

	if !workspaceID.Valid {
		p.logger.Warn(ctx, "chat has no workspace, cannot store recording")
		return ""
	}

	// The chatd actor is used here because the recording is stored on
	// behalf of the chat system, not a specific user request.
	//nolint:gocritic // AsChatd is required to read the workspace for org lookup.
	ws, err := p.db.GetWorkspaceByID(dbauthz.AsChatd(ctx), workspaceID.UUID)
	if err != nil {
		p.logger.Warn(ctx, "failed to resolve workspace for recording",
			slog.Error(err))
		return ""
	}

	//nolint:gocritic // AsChatd is required to insert chat files from the recording pipeline.
	row, err := p.db.InsertChatFile(dbauthz.AsChatd(ctx), database.InsertChatFileParams{
		OwnerID:        ownerID,
		OrganizationID: ws.OrganizationID,
		Name:           fmt.Sprintf("recording-%s.mp4", p.clock.Now().UTC().Format("2006-01-02T15-04-05Z")),
		Mimetype:       "video/mp4",
		Data:           data,
	})
	if err != nil {
		p.logger.Warn(ctx, "failed to store recording in database",
			slog.Error(err))
		return ""
	}
	return row.ID.String()
}
