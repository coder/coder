package toolsdk

import (
	"context"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/aisdk-go"
	"github.com/coder/coder/v2/codersdk"
)

const (
	ToolNameWorkspaceArtifactList = "coder_workspace_artifact_list"
	ToolNameWorkspaceArtifactRead = "coder_workspace_artifact_read"
)

// WorkspaceArtifactListArgs identify the immutable execution session.
type WorkspaceArtifactListArgs struct {
	OrganizationID string `json:"organization_id"`
	SessionID      string `json:"session_id"`
}

// WorkspaceArtifactReadArgs select a bounded range of preserved bytes.
type WorkspaceArtifactReadArgs struct {
	OrganizationID string `json:"organization_id"`
	SessionID      string `json:"session_id"`
	ArtifactID     string `json:"artifact_id"`
	Offset         int64  `json:"offset,omitempty"`
	Limit          int    `json:"limit,omitempty"`
}

// WorkspaceArtifactReadResult carries lossless bytes as JSON base64.
type WorkspaceArtifactReadResult struct {
	Content    []byte `json:"content"`
	Offset     int64  `json:"offset"`
	NextOffset int64  `json:"next_offset"`
	EOF        bool   `json:"eof"`
}

func artifactScope(organization, session string) (organizationID uuid.UUID, sessionID uuid.UUID, err error) {
	org, err := uuid.Parse(organization)
	if err != nil || org == uuid.Nil {
		return uuid.Nil, uuid.Nil, xerrors.New("organization_id must be a nonzero UUID")
	}
	id, err := uuid.Parse(session)
	if err != nil || id == uuid.Nil {
		return uuid.Nil, uuid.Nil, xerrors.New("session_id must be a nonzero UUID")
	}
	return org, id, nil
}

// WorkspaceArtifactList lists metadata and authenticated download references.
var WorkspaceArtifactList = Tool[WorkspaceArtifactListArgs, []codersdk.WorkspaceExecutionArtifact]{
	Tool: aisdk.Tool{
		Name: ToolNameWorkspaceArtifactList, Description: "List durable execution results, checksums, MIME types, explicit expiry, and authenticated download URLs. Available after the workspace is deleted.",
		Schema: aisdk.Schema{Properties: map[string]any{
			"organization_id": map[string]any{"type": "string", "format": "uuid"},
			"session_id":      map[string]any{"type": "string", "format": "uuid"},
		}, Required: []string{"organization_id", "session_id"}},
	},
	MCPAnnotations: mcpReadOnlyAnnotations,
	Handler: func(ctx context.Context, deps Deps, args WorkspaceArtifactListArgs) ([]codersdk.WorkspaceExecutionArtifact, error) {
		if deps.coderClient == nil {
			return nil, xerrors.New("artifact tools require an authenticated client")
		}
		org, session, err := artifactScope(args.OrganizationID, args.SessionID)
		if err != nil {
			return nil, err
		}
		return deps.coderClient.WorkspaceExecutionArtifacts(ctx, org, session)
	},
}

// WorkspaceArtifactRead retrieves an exact bounded range from durable storage.
var WorkspaceArtifactRead = Tool[WorkspaceArtifactReadArgs, WorkspaceArtifactReadResult]{
	Tool: aisdk.Tool{
		Name: ToolNameWorkspaceArtifactRead, Description: "Read preserved artifact bytes as base64, independent of workspace availability. Returns explicit next_offset and eof. Repeat with next_offset until eof; content is never silently truncated. Expired bytes return an error.",
		Schema: aisdk.Schema{Properties: map[string]any{
			"organization_id": map[string]any{"type": "string", "format": "uuid"},
			"session_id":      map[string]any{"type": "string", "format": "uuid"},
			"artifact_id":     map[string]any{"type": "string", "format": "uuid"},
			"offset":          map[string]any{"type": "integer", "minimum": 0},
			"limit":           map[string]any{"type": "integer", "minimum": 1, "maximum": 65536},
		}, Required: []string{"organization_id", "session_id", "artifact_id"}},
	},
	MCPAnnotations: mcpReadOnlyAnnotations,
	Handler: func(ctx context.Context, deps Deps, args WorkspaceArtifactReadArgs) (WorkspaceArtifactReadResult, error) {
		var result WorkspaceArtifactReadResult
		if deps.coderClient == nil {
			return result, xerrors.New("artifact tools require an authenticated client")
		}
		org, session, err := artifactScope(args.OrganizationID, args.SessionID)
		if err != nil {
			return result, err
		}
		id, err := uuid.Parse(args.ArtifactID)
		if err != nil || id == uuid.Nil {
			return result, xerrors.New("artifact_id must be a nonzero UUID")
		}
		if args.Limit == 0 {
			args.Limit = 65536
		}
		if args.Offset < 0 || args.Limit < 1 || args.Limit > 65536 {
			return result, xerrors.New("invalid artifact byte range")
		}
		chunk, err := deps.coderClient.ReadWorkspaceExecutionArtifact(ctx, org, session, id, args.Offset, args.Limit)
		if err != nil {
			return result, err
		}
		result.Content = chunk.Content
		result.Offset = args.Offset
		result.NextOffset = args.Offset + int64(len(result.Content))
		result.EOF = result.NextOffset == chunk.SizeBytes
		return result, nil
	},
}

// WorkspaceArtifactTools retrieve durable execution results.
var WorkspaceArtifactTools = []GenericTool{WorkspaceArtifactList.Generic(), WorkspaceArtifactRead.Generic()}
