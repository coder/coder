package chattool

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"strings"

	"charm.land/fantasy"
	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/codersdk/workspacesdk"
)

const maxAttachmentSize = 10 << 20 // 10 MiB

// StoreFileFunc persists a chat attachment after classifying it for durable
// storage and returns the stored attachment metadata.
type StoreFileFunc func(ctx context.Context, name string, detectName string, data []byte) (AttachmentMetadata, error)

// AttachmentMetadata identifies a durable chat attachment that should be
// promoted into a standard file message part for the user.
type AttachmentMetadata struct {
	FileID    uuid.UUID `json:"file_id"`
	MediaType string    `json:"media_type"`
	Name      string    `json:"name,omitempty"`
}

type attachmentResponseMetadata struct {
	Attachments []AttachmentMetadata `json:"attachments,omitempty"`
}

func storeAttachmentData(
	ctx context.Context,
	storeFile StoreFileFunc,
	name string,
	detectName string,
	data []byte,
) (AttachmentMetadata, error) {
	if storeFile == nil {
		return AttachmentMetadata{}, xerrors.New("file storage is not configured")
	}
	if len(data) == 0 {
		return AttachmentMetadata{}, xerrors.New("attachment is empty")
	}
	if len(data) > maxAttachmentSize {
		return AttachmentMetadata{}, xerrors.Errorf("attachment exceeds %d MiB size limit", maxAttachmentSize>>20)
	}

	name = strings.TrimSpace(name)
	if name == "" {
		return AttachmentMetadata{}, xerrors.New("attachment name is required")
	}
	if strings.TrimSpace(detectName) == "" {
		detectName = name
	}

	attachment, err := storeFile(ctx, name, detectName, data)
	if err != nil {
		return AttachmentMetadata{}, err
	}
	if attachment.FileID == uuid.Nil {
		return AttachmentMetadata{}, xerrors.New("stored attachment is missing file ID")
	}
	if attachment.MediaType == "" {
		return AttachmentMetadata{}, xerrors.New("stored attachment is missing media type")
	}
	if attachment.Name == "" {
		attachment.Name = name
	}
	return attachment, nil
}

func storeWorkspaceAttachment(
	ctx context.Context,
	conn workspacesdk.AgentConn,
	path string,
	name string,
	storeFile StoreFileFunc,
) (AttachmentMetadata, int, error) {
	if conn == nil {
		return AttachmentMetadata{}, 0, xerrors.New("workspace connection is not configured")
	}
	if strings.TrimSpace(path) == "" {
		return AttachmentMetadata{}, 0, xerrors.New("path is required")
	}
	reader, _, err := conn.ReadFile(ctx, path, 0, maxAttachmentSize+1)
	if err != nil {
		return AttachmentMetadata{}, 0, err
	}
	defer reader.Close()

	data, err := io.ReadAll(io.LimitReader(reader, maxAttachmentSize+1))
	if err != nil {
		return AttachmentMetadata{}, 0, err
	}
	if strings.TrimSpace(name) == "" {
		path = strings.TrimRight(path, "/\\")
		if idx := strings.LastIndexAny(path, "/\\"); idx >= 0 {
			name = path[idx+1:]
		} else {
			name = path
		}
	}
	attachment, err := storeAttachmentData(ctx, storeFile, name, path, data)
	if err != nil {
		return AttachmentMetadata{}, 0, err
	}
	return attachment, len(data), nil
}

func storeScreenshotAttachment(
	ctx context.Context,
	storeFile StoreFileFunc,
	name string,
	encodedPNG string,
) (AttachmentMetadata, error) {
	if strings.TrimSpace(encodedPNG) == "" {
		return AttachmentMetadata{}, xerrors.New("screenshot data is empty")
	}
	decoder := base64.NewDecoder(base64.StdEncoding, strings.NewReader(encodedPNG))
	data, err := io.ReadAll(io.LimitReader(decoder, maxAttachmentSize+1))
	if err != nil {
		return AttachmentMetadata{}, xerrors.Errorf("decode screenshot: %w", err)
	}
	if strings.TrimSpace(name) == "" {
		name = "screenshot.png"
	}
	return storeAttachmentData(ctx, storeFile, name, name, data)
}

// WithAttachments stores durable attachment metadata on a tool response so the
// persistence layer can promote the files into assistant chat attachments.
func WithAttachments(
	response fantasy.ToolResponse,
	attachments ...AttachmentMetadata,
) fantasy.ToolResponse {
	if len(attachments) == 0 {
		return response
	}
	return fantasy.WithResponseMetadata(response, attachmentResponseMetadata{
		Attachments: attachments,
	})
}

// AttachmentsFromMetadata decodes durable attachment metadata from a tool
// response so the persistence layer can promote them into assistant file parts.
func AttachmentsFromMetadata(metadata string) ([]AttachmentMetadata, error) {
	if strings.TrimSpace(metadata) == "" {
		return nil, nil
	}

	var decoded attachmentResponseMetadata
	if err := json.Unmarshal([]byte(metadata), &decoded); err != nil {
		return nil, xerrors.Errorf("unmarshal attachment metadata: %w", err)
	}

	attachments := make([]AttachmentMetadata, 0, len(decoded.Attachments))
	for i, attachment := range decoded.Attachments {
		if attachment.FileID == uuid.Nil {
			return nil, xerrors.Errorf("attachment %d is missing file_id", i)
		}
		if attachment.MediaType == "" {
			return nil, xerrors.Errorf("attachment %d is missing media_type", i)
		}
		attachments = append(attachments, attachment)
	}
	return attachments, nil
}
