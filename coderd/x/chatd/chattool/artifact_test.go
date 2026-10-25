package chattool_test

import (
	"context"
	"encoding/json"
	"io"
	"strings"
	"testing"

	"charm.land/fantasy"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/coder/coder/v2/coderd/x/chatd/chattool"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk/agentconnmock"
)

// fakeArtifactStore records the publish it gets and returns fixed results.
type fakeArtifactStore struct {
	published  chattool.PublishArtifactParams
	publishErr error
	content    chattool.ArtifactContent
	readErr    error
	readID     uuid.UUID
	readVer    int32
}

func (s *fakeArtifactStore) Publish(_ context.Context, params chattool.PublishArtifactParams) (chattool.PublishedArtifact, error) {
	s.published = params
	if s.publishErr != nil {
		return chattool.PublishedArtifact{}, s.publishErr
	}
	id := params.ArtifactID.UUID
	if !params.ArtifactID.Valid {
		id = uuid.MustParse("11111111-2222-3333-4444-555555555555")
	}
	title := params.Title
	if title == "" {
		title = "Existing title"
	}
	return chattool.PublishedArtifact{ArtifactID: id, Version: params.BaseVersion + 1, Title: title, Name: params.Name}, nil
}

func (s *fakeArtifactStore) Read(_ context.Context, artifactID uuid.UUID, version int32) (chattool.ArtifactContent, error) {
	s.readID = artifactID
	s.readVer = version
	return s.content, s.readErr
}

func artifactTools(conn workspacesdk.AgentConn, store chattool.ArtifactStore) (publish, read fantasy.AgentTool) {
	opts := chattool.ArtifactOptions{
		GetWorkspaceConn: func(context.Context) (workspacesdk.AgentConn, error) { return conn, nil },
		Store:            store,
	}
	return chattool.PublishArtifact(opts), chattool.ReadArtifact(opts)
}

func runArtifactTool(t *testing.T, tool fantasy.AgentTool, input any) (fantasy.ToolResponse, map[string]any) {
	t.Helper()
	raw, err := json.Marshal(input)
	require.NoError(t, err)
	resp, err := tool.Run(context.Background(), fantasy.ToolCall{ID: "call-1", Name: tool.Info().Name, Input: string(raw)})
	require.NoError(t, err)
	if resp.IsError {
		return resp, nil
	}
	var out map[string]any
	require.NoError(t, json.Unmarshal([]byte(resp.Content), &out))
	return resp, out
}

func TestPublishArtifact(t *testing.T) {
	t.Parallel()

	expectRead := func(conn *agentconnmock.MockAgentConn, path, content string) {
		conn.EXPECT().ReadFile(gomock.Any(), path, int64(0), int64(10<<20+1)).
			Return(io.NopCloser(strings.NewReader(content)), "text/html", nil)
	}

	t.Run("CreateDefaultsTitleToFileName", func(t *testing.T) {
		t.Parallel()
		conn := agentconnmock.NewMockAgentConn(gomock.NewController(t))
		expectRead(conn, "/home/coder/funnel-report.html", "<h1>hi</h1>")
		store := &fakeArtifactStore{}
		publish, _ := artifactTools(conn, store)

		resp, out := runArtifactTool(t, publish, map[string]any{"path": "/home/coder/funnel-report.html", "label": "first"})
		require.False(t, resp.IsError, resp.Content)
		require.False(t, store.published.ArtifactID.Valid)
		require.Equal(t, int32(0), store.published.BaseVersion)
		require.Equal(t, "funnel-report", store.published.Title)
		require.Equal(t, "first", store.published.Label)
		require.Equal(t, "funnel-report.html", store.published.Name)
		require.Equal(t, []byte("<h1>hi</h1>"), store.published.Data)
		require.Equal(t, map[string]any{
			"ok":          true,
			"artifact_id": "11111111-2222-3333-4444-555555555555",
			"version":     float64(1),
			"title":       "funnel-report",
			"name":        "funnel-report.html",
			"url":         "/agents/artifacts/11111111-2222-3333-4444-555555555555",
		}, out)
	})

	t.Run("NextVersionKeepsTitle", func(t *testing.T) {
		t.Parallel()
		conn := agentconnmock.NewMockAgentConn(gomock.NewController(t))
		expectRead(conn, "/tmp/report.md", "# v2")
		store := &fakeArtifactStore{}
		publish, _ := artifactTools(conn, store)
		id := uuid.New()

		resp, out := runArtifactTool(t, publish, map[string]any{"path": "/tmp/report.md", "artifact_id": id.String(), "base_version": 1})
		require.False(t, resp.IsError, resp.Content)
		require.Equal(t, uuid.NullUUID{UUID: id, Valid: true}, store.published.ArtifactID)
		require.Equal(t, int32(1), store.published.BaseVersion)
		require.Empty(t, store.published.Title, "an update without title keeps the title")
		require.Equal(t, float64(2), out["version"])
		require.Equal(t, "/agents/artifacts/"+id.String(), out["url"])
	})

	t.Run("StaleBaseNamesLatest", func(t *testing.T) {
		t.Parallel()
		conn := agentconnmock.NewMockAgentConn(gomock.NewController(t))
		expectRead(conn, "/tmp/report.md", "# v2")
		store := &fakeArtifactStore{publishErr: &chattool.StaleBaseError{Latest: 3}}
		publish, _ := artifactTools(conn, store)

		resp, _ := runArtifactTool(t, publish, map[string]any{"path": "/tmp/report.md", "artifact_id": uuid.NewString(), "base_version": 1})
		require.True(t, resp.IsError)
		require.Contains(t, resp.Content, "v3 is the latest")
		require.Contains(t, resp.Content, "base_version 3")
	})

	t.Run("ArtifactIDNeedsBaseVersion", func(t *testing.T) {
		t.Parallel()
		conn := agentconnmock.NewMockAgentConn(gomock.NewController(t))
		publish, _ := artifactTools(conn, &fakeArtifactStore{})
		resp, _ := runArtifactTool(t, publish, map[string]any{"path": "/tmp/report.md", "artifact_id": uuid.NewString()})
		require.True(t, resp.IsError)
		require.Contains(t, resp.Content, "base_version is required")
	})

	t.Run("RejectsBadArtifactID", func(t *testing.T) {
		t.Parallel()
		conn := agentconnmock.NewMockAgentConn(gomock.NewController(t))
		publish, _ := artifactTools(conn, &fakeArtifactStore{})
		resp, _ := runArtifactTool(t, publish, map[string]any{"path": "/tmp/report.md", "artifact_id": "7f3c2a", "base_version": 1})
		require.True(t, resp.IsError)
		require.Contains(t, resp.Content, "UUID")
	})

	t.Run("RejectsOversizedFile", func(t *testing.T) {
		t.Parallel()
		conn := agentconnmock.NewMockAgentConn(gomock.NewController(t))
		expectRead(conn, "/tmp/big.txt", strings.Repeat("a", 10<<20+1))
		store := &fakeArtifactStore{}
		publish, _ := artifactTools(conn, store)
		resp, _ := runArtifactTool(t, publish, map[string]any{"path": "/tmp/big.txt"})
		require.True(t, resp.IsError)
		require.Contains(t, resp.Content, "10 MiB")
		require.Nil(t, store.published.Data)
	})
}

func TestReadArtifact(t *testing.T) {
	t.Parallel()

	t.Run("WritesLatestIntoWorkspace", func(t *testing.T) {
		t.Parallel()
		id := uuid.New()
		conn := agentconnmock.NewMockAgentConn(gomock.NewController(t))
		wantPath := "/tmp/coder-artifacts/" + id.String() + "/funnel-report.html"
		var written []byte
		conn.EXPECT().WriteFile(gomock.Any(), wantPath, gomock.Any()).
			DoAndReturn(func(_ context.Context, _ string, r io.Reader) error {
				var err error
				written, err = io.ReadAll(r)
				return err
			})
		store := &fakeArtifactStore{content: chattool.ArtifactContent{
			ArtifactID: id, Version: 2, Title: "Signup funnel", Name: "funnel-report.html", Data: []byte("<p>v2</p>"), Untrusted: true,
		}}
		_, read := artifactTools(conn, store)

		resp, out := runArtifactTool(t, read, map[string]any{"artifact_id": id.String()})
		require.False(t, resp.IsError, resp.Content)
		require.Equal(t, id, store.readID)
		require.Equal(t, int32(0), store.readVer, "no version reads the latest")
		require.Equal(t, []byte("<p>v2</p>"), written)
		require.Equal(t, map[string]any{
			"ok":          true,
			"artifact_id": id.String(),
			"version":     float64(2),
			"title":       "Signup funnel",
			"name":        "funnel-report.html",
			"path":        wantPath,
			"untrusted":   true,
		}, out)
	})

	t.Run("NameCannotLeaveDirectory", func(t *testing.T) {
		t.Parallel()
		id := uuid.New()
		conn := agentconnmock.NewMockAgentConn(gomock.NewController(t))
		conn.EXPECT().WriteFile(gomock.Any(), "/tmp/coder-artifacts/"+id.String()+"/.._etc_passwd", gomock.Any()).Return(nil)
		store := &fakeArtifactStore{content: chattool.ArtifactContent{ArtifactID: id, Version: 1, Name: "../etc/passwd", Data: []byte("x")}}
		_, read := artifactTools(conn, store)
		resp, out := runArtifactTool(t, read, map[string]any{"artifact_id": id.String(), "version": 1})
		require.False(t, resp.IsError, resp.Content)
		require.Equal(t, int32(1), store.readVer)
		require.Equal(t, false, out["untrusted"])
	})

	t.Run("NotFound", func(t *testing.T) {
		t.Parallel()
		conn := agentconnmock.NewMockAgentConn(gomock.NewController(t))
		_, read := artifactTools(conn, &fakeArtifactStore{readErr: chattool.ErrArtifactNotFound})
		resp, _ := runArtifactTool(t, read, map[string]any{"artifact_id": uuid.NewString()})
		require.True(t, resp.IsError)
		require.Contains(t, resp.Content, "artifact not found")
	})
}
