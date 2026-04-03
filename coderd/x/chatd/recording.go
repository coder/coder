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
// failure. All errors are logged but not propagated; recording is
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
	// First pass: parse all multipart parts into memory.
	// The agent sends at most two parts: one video/mp4 and one
	// optional image/jpeg thumbnail. Cap the number of parts to
	// prevent a malicious or broken agent from forcing the server
	// into an unbounded parsing loop.
	const maxParts = 2
	var videoData, thumbnailData []byte
	for range maxParts {
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

		// Select the read limit based on content type so that
		// thumbnails (image/jpeg) do not allocate up to
		// MaxRecordingSize (100 MB) before the size check rejects
		// them. Unknown types use a small default since they are
		// discarded below.
		maxSize := int64(1 << 20) // 1 MB default for unknown types
		switch contentType {
		case "video/mp4":
			maxSize = int64(workspacesdk.MaxRecordingSize)
		case "image/jpeg":
			maxSize = int64(workspacesdk.MaxThumbnailSize)
		}

		data, err := io.ReadAll(io.LimitReader(part, maxSize+1))
		if err != nil {
			p.logger.Warn(ctx, "failed to read recording part data",
				slog.F("content_type", contentType),
				slog.Error(err))
			continue
		}
		if int64(len(data)) > maxSize {
			p.logger.Warn(ctx, "recording part exceeds maximum size, skipping",
				slog.F("content_type", contentType),
				slog.F("size", len(data)),
				slog.F("max_size", maxSize))
			continue
		}
		if len(data) == 0 {
			p.logger.Warn(ctx, "recording part is empty, skipping",
				slog.F("content_type", contentType))
			continue
		}

		switch contentType {
		case "video/mp4":
			if videoData != nil {
				p.logger.Warn(ctx, "duplicate video/mp4 part in recording response, skipping")
				continue
			}
			videoData = data
		case "image/jpeg":
			if thumbnailData != nil {
				p.logger.Warn(ctx, "duplicate image/jpeg part in recording response, skipping")
				continue
			}
			thumbnailData = data
		default:
			p.logger.Debug(ctx, "skipping unknown part content type",
				slog.F("content_type", contentType))
		}
	}

	// Second pass: store the collected data in the database.
	if videoData != nil {
		//nolint:gocritic // AsChatd is required to insert chat files from the recording pipeline.
		row, err := p.db.InsertChatFile(dbauthz.AsChatd(ctx), database.InsertChatFileParams{
			OwnerID:        ownerID,
			OrganizationID: ws.OrganizationID,
			Name:           fmt.Sprintf("recording-%s.mp4", p.clock.Now().UTC().Format("2006-01-02T15-04-05Z")),
			Mimetype:       "video/mp4",
			Data:           videoData,
		})
		if err != nil {
			p.logger.Warn(ctx, "failed to store recording in database",
				slog.Error(err))
		} else {
			result.recordingFileID = row.ID.String()
		}
	}
	if thumbnailData != nil && result.recordingFileID != "" {
		//nolint:gocritic // AsChatd is required to insert chat files from the recording pipeline.
		row, err := p.db.InsertChatFile(dbauthz.AsChatd(ctx), database.InsertChatFileParams{
			OwnerID:        ownerID,
			OrganizationID: ws.OrganizationID,
			Name:           fmt.Sprintf("thumbnail-%s.jpg", p.clock.Now().UTC().Format("2006-01-02T15-04-05Z")),
			Mimetype:       "image/jpeg",
			Data:           thumbnailData,
		})
		if err != nil {
			p.logger.Warn(ctx, "failed to store thumbnail in database",
				slog.Error(err))
		} else {
			result.thumbnailFileID = row.ID.String()
		}
	}

	return result
}
