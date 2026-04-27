package chattool_test

import (
	"context"
	"encoding/json"
	"io"
	"strings"
	"testing"

	"charm.land/fantasy"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/x/chatd/chattool"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk/agentconnmock"
)

type attachFileResponse struct {
	OK        bool   `json:"ok"`
	Path      string `json:"path"`
	FileID    string `json:"file_id"`
	Name      string `json:"name"`
	MediaType string `json:"media_type"`
	Size      int    `json:"size"`
}

func TestAttachFile(t *testing.T) {
	t.Parallel()

	t.Run("EmptyPathReturnsError", func(t *testing.T) {
		t.Parallel()
		ctrl := gomock.NewController(t)
		mockConn := agentconnmock.NewMockAgentConn(ctrl)
		tool := newAttachFileTool(t, mockConn, func(_ context.Context, _ string, _ string, _ []byte) (chattool.AttachmentMetadata, error) {
			return chattool.AttachmentMetadata{}, nil
		})
		resp, err := tool.Run(context.Background(), fantasy.ToolCall{
			ID: "call-1", Name: "attach_file", Input: `{"path":""}`,
		})
		require.NoError(t, err)
		assert.True(t, resp.IsError)
		assert.Contains(t, resp.Content, "path is required")
	})

	t.Run("RelativePathErrorComesFromAgent", func(t *testing.T) {
		t.Parallel()
		ctrl := gomock.NewController(t)
		mockConn := agentconnmock.NewMockAgentConn(ctrl)
		mockConn.EXPECT().
			ReadFile(gomock.Any(), "notes.txt", int64(0), int64(10<<20+1)).
			Return(nil, "", xerrors.New(`file path must be absolute: "notes.txt"`))
		tool := newAttachFileTool(t, mockConn, func(_ context.Context, _ string, _ string, _ []byte) (chattool.AttachmentMetadata, error) {
			return chattool.AttachmentMetadata{}, nil
		})
		resp, err := tool.Run(context.Background(), fantasy.ToolCall{
			ID: "call-1", Name: "attach_file", Input: `{"path":"notes.txt"}`,
		})
		require.NoError(t, err)
		assert.True(t, resp.IsError)
		assert.Contains(t, resp.Content, `file path must be absolute: "notes.txt"`)
	})

	t.Run("ValidTextFileStoresAttachment", func(t *testing.T) {
		t.Parallel()
		ctrl := gomock.NewController(t)
		mockConn := agentconnmock.NewMockAgentConn(ctrl)
		content := "build succeeded\n"
		mockConn.EXPECT().
			ReadFile(gomock.Any(), "/home/coder/build.log", int64(0), int64(10<<20+1)).
			Return(io.NopCloser(strings.NewReader(content)), "text/plain", nil)

		var storedName string
		var storedType string
		var storedData []byte
		tool := newAttachFileTool(t, mockConn, func(_ context.Context, name string, detectName string, data []byte) (chattool.AttachmentMetadata, error) {
			storedName = name
			require.Equal(t, "/home/coder/build.log", detectName)
			storedType = "text/plain"
			storedData = append([]byte(nil), data...)
			return chattool.AttachmentMetadata{
				FileID:    uuid.MustParse("aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"),
				MediaType: storedType,
				Name:      name,
			}, nil
		})
		resp, err := tool.Run(context.Background(), fantasy.ToolCall{
			ID: "call-1", Name: "attach_file", Input: `{"path":"/home/coder/build.log"}`,
		})
		require.NoError(t, err)
		assert.False(t, resp.IsError)
		assert.Equal(t, "build.log", storedName)
		assert.Equal(t, "text/plain", storedType)
		assert.Equal(t, []byte(content), storedData)

		decoded := decodeAttachFileResponse(t, resp)
		assert.True(t, decoded.OK)
		assert.Equal(t, "/home/coder/build.log", decoded.Path)
		assert.Equal(t, "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee", decoded.FileID)
		assert.Equal(t, "build.log", decoded.Name)
		assert.Equal(t, "text/plain", decoded.MediaType)
		assert.Equal(t, len(content), decoded.Size)

		attachments, err := chattool.AttachmentsFromMetadata(resp.Metadata)
		require.NoError(t, err)
		require.Len(t, attachments, 1)
		assert.Equal(t, uuid.MustParse(decoded.FileID), attachments[0].FileID)
		assert.Equal(t, decoded.MediaType, attachments[0].MediaType)
		assert.Equal(t, decoded.Name, attachments[0].Name)
	})

	t.Run("WindowsAbsolutePathUsesBaseName", func(t *testing.T) {
		t.Parallel()
		ctrl := gomock.NewController(t)
		mockConn := agentconnmock.NewMockAgentConn(ctrl)
		content := "build succeeded\n"
		path := `C:\Users\coder\build.log`
		mockConn.EXPECT().
			ReadFile(gomock.Any(), path, int64(0), int64(10<<20+1)).
			Return(io.NopCloser(strings.NewReader(content)), "text/plain", nil)

		var storedName string
		tool := newAttachFileTool(t, mockConn, func(_ context.Context, name string, detectName string, data []byte) (chattool.AttachmentMetadata, error) {
			storedName = name
			require.Equal(t, path, detectName)
			assert.Equal(t, []byte(content), data)
			return chattool.AttachmentMetadata{
				FileID:    uuid.MustParse("dddddddd-eeee-ffff-0000-111111111111"),
				MediaType: "text/plain",
				Name:      name,
			}, nil
		})
		input, err := json.Marshal(chattool.AttachFileArgs{Path: path})
		require.NoError(t, err)
		resp, err := tool.Run(context.Background(), fantasy.ToolCall{
			ID:    "call-windows",
			Name:  "attach_file",
			Input: string(input),
		})
		require.NoError(t, err)
		assert.False(t, resp.IsError)
		assert.Equal(t, "build.log", storedName)

		decoded := decodeAttachFileResponse(t, resp)
		assert.Equal(t, path, decoded.Path)
		assert.Equal(t, "build.log", decoded.Name)
		assert.Equal(t, len(content), decoded.Size)
	})

	t.Run("CustomNameOverridePreservesJSONSubtype", func(t *testing.T) {
		t.Parallel()
		ctrl := gomock.NewController(t)
		mockConn := agentconnmock.NewMockAgentConn(ctrl)
		content := `{"ok":true}`
		mockConn.EXPECT().
			ReadFile(gomock.Any(), "/home/coder/report.json", int64(0), int64(10<<20+1)).
			Return(io.NopCloser(strings.NewReader(content)), "text/plain", nil)

		var storedName string
		var storedType string
		tool := newAttachFileTool(t, mockConn, func(_ context.Context, name string, detectName string, data []byte) (chattool.AttachmentMetadata, error) {
			storedName = name
			require.Equal(t, "/home/coder/report.json", detectName)
			storedType = "application/json"
			assert.Equal(t, []byte(content), data)
			return chattool.AttachmentMetadata{
				FileID:    uuid.MustParse("bbbbbbbb-cccc-dddd-eeee-ffffffffffff"),
				MediaType: storedType,
				Name:      name,
			}, nil
		})
		resp, err := tool.Run(context.Background(), fantasy.ToolCall{
			ID: "call-json", Name: "attach_file", Input: `{"path":"/home/coder/report.json","name":"payload.txt"}`,
		})
		require.NoError(t, err)
		assert.False(t, resp.IsError)
		assert.Equal(t, "payload.txt", storedName)
		assert.Equal(t, "application/json", storedType)

		decoded := decodeAttachFileResponse(t, resp)
		assert.Equal(t, "payload.txt", decoded.Name)
		assert.Equal(t, "application/json", decoded.MediaType)
		assert.Equal(t, len(content), decoded.Size)
	})

	t.Run("EmptyFileRejected", func(t *testing.T) {
		t.Parallel()
		ctrl := gomock.NewController(t)
		mockConn := agentconnmock.NewMockAgentConn(ctrl)
		mockConn.EXPECT().
			ReadFile(gomock.Any(), "/home/coder/empty.txt", int64(0), int64(10<<20+1)).
			Return(io.NopCloser(strings.NewReader("")), "text/plain", nil)

		tool := newAttachFileTool(t, mockConn, func(_ context.Context, _ string, _ string, _ []byte) (chattool.AttachmentMetadata, error) {
			t.Fatal("storeFile should not be called for empty attachments")
			return chattool.AttachmentMetadata{}, nil
		})
		resp, err := tool.Run(context.Background(), fantasy.ToolCall{
			ID: "call-empty", Name: "attach_file", Input: `{"path":"/home/coder/empty.txt"}`,
		})
		require.NoError(t, err)
		assert.True(t, resp.IsError)
		assert.Contains(t, resp.Content, "attachment is empty")
		attachments, err := chattool.AttachmentsFromMetadata(resp.Metadata)
		require.NoError(t, err)
		assert.Empty(t, attachments)
	})

	t.Run("OversizedFileRejected", func(t *testing.T) {
		t.Parallel()
		ctrl := gomock.NewController(t)
		mockConn := agentconnmock.NewMockAgentConn(ctrl)
		largeContent := strings.Repeat("x", 10<<20+1)
		mockConn.EXPECT().
			ReadFile(gomock.Any(), "/home/coder/build.log", int64(0), int64(10<<20+1)).
			Return(io.NopCloser(strings.NewReader(largeContent)), "text/plain", nil)

		tool := newAttachFileTool(t, mockConn, func(_ context.Context, _ string, _ string, _ []byte) (chattool.AttachmentMetadata, error) {
			return chattool.AttachmentMetadata{}, xerrors.New("should not be called")
		})
		resp, err := tool.Run(context.Background(), fantasy.ToolCall{
			ID: "call-1", Name: "attach_file", Input: `{"path":"/home/coder/build.log"}`,
		})
		require.NoError(t, err)
		assert.True(t, resp.IsError)
		assert.Contains(t, resp.Content, "attachment exceeds 10 MiB size limit")
	})

	t.Run("ReadFileError", func(t *testing.T) {
		t.Parallel()
		ctrl := gomock.NewController(t)
		mockConn := agentconnmock.NewMockAgentConn(ctrl)
		mockConn.EXPECT().
			ReadFile(gomock.Any(), "/home/coder/build.log", int64(0), int64(10<<20+1)).
			Return(nil, "", xerrors.New("file not found"))

		tool := newAttachFileTool(t, mockConn, func(_ context.Context, _ string, _ string, _ []byte) (chattool.AttachmentMetadata, error) {
			return chattool.AttachmentMetadata{}, nil
		})
		resp, err := tool.Run(context.Background(), fantasy.ToolCall{
			ID: "call-1", Name: "attach_file", Input: `{"path":"/home/coder/build.log"}`,
		})
		require.NoError(t, err)
		assert.True(t, resp.IsError)
		assert.Contains(t, resp.Content, "file not found")
	})

	t.Run("StoreFileErrorSurfaces", func(t *testing.T) {
		t.Parallel()
		ctrl := gomock.NewController(t)
		mockConn := agentconnmock.NewMockAgentConn(ctrl)
		mockConn.EXPECT().
			ReadFile(gomock.Any(), "/home/coder/build.log", int64(0), int64(10<<20+1)).
			Return(io.NopCloser(strings.NewReader("build succeeded\n")), "text/plain", nil)

		tool := newAttachFileTool(t, mockConn, func(_ context.Context, _ string, _ string, _ []byte) (chattool.AttachmentMetadata, error) {
			return chattool.AttachmentMetadata{}, xerrors.New("chat already has the maximum of 20 linked files")
		})
		resp, err := tool.Run(context.Background(), fantasy.ToolCall{
			ID: "call-cap", Name: "attach_file", Input: `{"path":"/home/coder/build.log"}`,
		})
		require.NoError(t, err)
		assert.True(t, resp.IsError)
		assert.Contains(t, resp.Content, "chat already has the maximum of 20 linked files")
	})
}

func newAttachFileTool(
	t *testing.T,
	mockConn *agentconnmock.MockAgentConn,
	storeFile chattool.StoreFileFunc,
) fantasy.AgentTool {
	t.Helper()
	return chattool.AttachFile(chattool.AttachFileOptions{
		GetWorkspaceConn: func(_ context.Context) (workspacesdk.AgentConn, error) {
			return mockConn, nil
		},
		StoreFile: storeFile,
	})
}

func decodeAttachFileResponse(t *testing.T, resp fantasy.ToolResponse) attachFileResponse {
	t.Helper()
	var result attachFileResponse
	require.NoError(t, json.Unmarshal([]byte(resp.Content), &result))
	return result
}
