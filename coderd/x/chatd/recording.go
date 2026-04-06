package chatd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"mime"
	"mime/multipart"

	"github.com/google/uuid"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
)

type recordingResult struct {
	recordingFileID string
	thumbnailFileID string
}

// stopAndStoreRecording stops the desktop recording, downloads the
// multipart response containing the MP4 and optional thumbnail, and
// stores them in chat_files. Only called when the subagent completed
// successfully. Returns file IDs on success, empty fields on any
// failure. All errors are logged but not propagated — recording is
// best-effort.
func (p *Server) stopAndStoreRecording(
	ctx context.Context,
	conn workspacesdk.AgentConn,
	recordingID string,
	ownerID uuid.UUID,
	workspaceID uuid.NullUUID,
) recordingResult {
	var result recordingResult

	select {
	case p.recordingSem <- struct{}{}:
		defer func() { <-p.recordingSem }()
	case <-ctx.Done():
		p.logger.Warn(ctx, "context canceled waiting for recording semaphore", slog.Error(ctx.Err()))
		return result
	}

	resp, err := conn.StopDesktopRecording(ctx,
		workspacesdk.StopDesktopRecordingRequest{RecordingID: recordingID})
	if err != nil {
		p.logger.Warn(ctx, "failed to stop desktop recording",
			slog.Error(err))
		return result
	}
	defer resp.Body.Close()

	_, params, err := mime.ParseMediaType(resp.ContentType)
	if err != nil {
		p.logger.Warn(ctx, "failed to parse content type from recording response",
			slog.F("content_type", resp.ContentType),
			slog.Error(err))
		return result
	}
	boundary := params["boundary"]
	if boundary == "" {
		p.logger.Warn(ctx, "missing boundary in recording response content type",
			slog.F("content_type", resp.ContentType))
		return result
	}

	if !workspaceID.Valid {
		p.logger.Warn(ctx, "chat has no workspace, cannot store recording")
		return result
	}

	// The chatd actor is used here because the recording is stored on
	// behalf of the chat system, not a specific user request.
	//nolint:gocritic // AsChatd is required to read the workspace for org lookup.
	ws, err := p.db.GetWorkspaceByID(dbauthz.AsChatd(ctx), workspaceID.UUID)
	if err != nil {
		p.logger.Warn(ctx, "failed to resolve workspace for recording",
			slog.Error(err))
		return result
	}

	mr := multipart.NewReader(resp.Body, boundary)
	// Context cancellation is checked between parts. Within a
	// part read, cancellation relies on Go's HTTP transport closing
	// the underlying connection when the context is done, which
	// interrupts the blocked io.ReadAll.
	for {
		if ctx.Err() != nil {
			p.logger.Warn(ctx, "context canceled while reading recording parts", slog.Error(ctx.Err()))
			break
		}

		part, err := mr.NextPart()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			p.logger.Warn(ctx, "error reading next multipart part", slog.Error(err))
			break
		}

		contentType := part.Header.Get("Content-Type")

		data, err := io.ReadAll(io.LimitReader(part, workspacesdk.MaxRecordingSize+1))
		if err != nil {
			p.logger.Warn(ctx, "failed to read recording part data",
				slog.F("content_type", contentType),
				slog.Error(err))
			continue
		}
		if len(data) > workspacesdk.MaxRecordingSize {
			p.logger.Warn(ctx, "recording part exceeds maximum size, skipping",
				slog.F("content_type", contentType),
				slog.F("size", len(data)),
				slog.F("max_size", workspacesdk.MaxRecordingSize))
			continue
		}
		if len(data) == 0 {
			p.logger.Warn(ctx, "recording part is empty, skipping",
				slog.F("content_type", contentType))
			continue
		}

		switch contentType {
		case "video/mp4":
			if result.recordingFileID != "" {
				p.logger.Warn(ctx, "duplicate video/mp4 part in recording response, skipping")
				continue
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
				continue
			}
			result.recordingFileID = row.ID.String()
		case "image/jpeg":
			if result.thumbnailFileID != "" {
				p.logger.Warn(ctx, "duplicate image/jpeg part in recording response, skipping")
				continue
			}
			if len(data) > workspacesdk.MaxThumbnailSize {
				p.logger.Warn(ctx, "thumbnail part exceeds maximum size, skipping",
					slog.F("size", len(data)),
					slog.F("max_size", workspacesdk.MaxThumbnailSize))
				continue
			}
			//nolint:gocritic // AsChatd is required to insert chat files from the recording pipeline.
			row, err := p.db.InsertChatFile(dbauthz.AsChatd(ctx), database.InsertChatFileParams{
				OwnerID:        ownerID,
				OrganizationID: ws.OrganizationID,
				Name:           fmt.Sprintf("thumbnail-%s.jpg", p.clock.Now().UTC().Format("2006-01-02T15-04-05Z")),
				Mimetype:       "image/jpeg",
				Data:           data,
			})
			if err != nil {
				p.logger.Warn(ctx, "failed to store thumbnail in database",
					slog.Error(err))
				continue
			}
			result.thumbnailFileID = row.ID.String()
		default:
			p.logger.Debug(ctx, "skipping unknown part content type",
				slog.F("content_type", contentType))
		}
	}

	return result
}
