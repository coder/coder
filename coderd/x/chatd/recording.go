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
	"github.com/coder/coder/v2/coderd/x/chatd/chattool"
	"github.com/coder/coder/v2/coderd/x/chatfiles"
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
	parentChatID uuid.UUID,
	ownerID uuid.UUID,
	workspaceID uuid.NullUUID,
) recordingResult {
	var result recordingResult

	workspaceIDValue := ""
	if workspaceID.Valid {
		workspaceIDValue = workspaceID.UUID.String()
	}
	recordingWarnFields := []slog.Field{
		slog.F("recording_id", recordingID),
		slog.F("parent_chat_id", parentChatID.String()),
		slog.F("workspace_id", workspaceIDValue),
	}
	warn := func(msg string, fields ...slog.Field) {
		allFields := make([]slog.Field, 0, len(recordingWarnFields)+len(fields))
		allFields = append(allFields, recordingWarnFields...)
		allFields = append(allFields, fields...)
		p.logger.Warn(ctx, msg, allFields...)
	}

	select {
	case p.recordingSem <- struct{}{}:
		defer func() { <-p.recordingSem }()
	case <-ctx.Done():
		warn("context canceled waiting for recording semaphore", slog.Error(ctx.Err()))
		return result
	}

	resp, err := conn.StopDesktopRecording(ctx,
		workspacesdk.StopDesktopRecordingRequest{RecordingID: recordingID})
	if err != nil {
		warn("failed to stop desktop recording",
			slog.Error(err))
		return result
	}
	defer resp.Body.Close()

	_, params, err := mime.ParseMediaType(resp.ContentType)
	if err != nil {
		warn("failed to parse content type from recording response",
			slog.F("content_type", resp.ContentType),
			slog.Error(err))
		return result
	}
	boundary := params["boundary"]
	if boundary == "" {
		warn("missing boundary in recording response content type",
			slog.F("content_type", resp.ContentType))
		return result
	}

	if !workspaceID.Valid {
		warn("chat has no workspace, cannot store recording")
		return result
	}

	// The chatd actor is used here because the recording is stored on
	// behalf of the chat system, not a specific user request.
	//nolint:gocritic // AsChatd is required to read the workspace for org lookup.
	chatdCtx := dbauthz.AsChatd(ctx)
	ws, err := p.db.GetWorkspaceByID(chatdCtx, workspaceID.UUID)
	if err != nil {
		warn("failed to resolve workspace for recording",
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
			warn("context canceled while reading recording parts", slog.Error(ctx.Err()))
			break
		}

		part, err := mr.NextPart()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			warn("error reading next multipart part", slog.Error(err))
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
			warn("failed to read recording part data",
				slog.F("content_type", contentType),
				slog.Error(err))
			continue
		}
		if int64(len(data)) > maxSize {
			warn("recording part exceeds maximum size, skipping",
				slog.F("content_type", contentType),
				slog.F("size", len(data)),
				slog.F("max_size", maxSize))
			continue
		}
		if len(data) == 0 {
			warn("recording part is empty, skipping",
				slog.F("content_type", contentType))
			continue
		}

		switch contentType {
		case "video/mp4":
			if videoData != nil {
				warn("duplicate video/mp4 part in recording response, skipping")
				continue
			}
			videoData = data
		case "image/jpeg":
			if thumbnailData != nil {
				warn("duplicate image/jpeg part in recording response, skipping")
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
		attachment, err := p.storeRecordingArtifact(
			chatdCtx,
			parentChatID,
			ownerID,
			ws.OrganizationID,
			fmt.Sprintf("recording-%s.mp4", p.clock.Now().UTC().Format("2006-01-02T15-04-05Z")),
			"video/mp4",
			videoData,
		)
		if err != nil {
			warn("failed to store recording in database",
				slog.Error(err))
		} else {
			result.recordingFileID = attachment.FileID.String()
		}
	}
	if thumbnailData != nil && result.recordingFileID != "" {
		attachment, err := p.storeRecordingArtifact(
			chatdCtx,
			parentChatID,
			ownerID,
			ws.OrganizationID,
			fmt.Sprintf("thumbnail-%s.jpg", p.clock.Now().UTC().Format("2006-01-02T15-04-05Z")),
			"image/jpeg",
			thumbnailData,
		)
		if err != nil {
			warn("failed to store thumbnail in database",
				slog.Error(err))
		} else {
			result.thumbnailFileID = attachment.FileID.String()
		}
	}

	return result
}

func (p *Server) storeRecordingArtifact(
	ctx context.Context,
	chatID uuid.UUID,
	ownerID uuid.UUID,
	organizationID uuid.UUID,
	name string,
	mediaType string,
	data []byte,
) (chattool.AttachmentMetadata, error) {
	storedName, verifiedMediaType, err := chatfiles.PrepareRecordingArtifact(name, mediaType, data)
	if err != nil {
		return chattool.AttachmentMetadata{}, err
	}

	var attachment chattool.AttachmentMetadata
	err = p.db.InTx(func(tx database.Store) error {
		var err error
		attachment, err = storeLinkedChatFileTx(
			ctx,
			tx,
			chatID,
			ownerID,
			organizationID,
			storedName,
			verifiedMediaType,
			data,
		)
		return err
	}, database.DefaultTXOptions().WithID("store_recording_artifact"))
	if err != nil {
		return chattool.AttachmentMetadata{}, err
	}
	return attachment, nil
}
