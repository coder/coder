package codersdk

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/google/uuid"
	"golang.org/x/xerrors"
)

// WorkspaceExecutionArtifact describes durable bytes independent of workspace lifetime.
type WorkspaceExecutionArtifact struct {
	ID                   uuid.UUID  `json:"id" format:"uuid"`
	OrganizationID       uuid.UUID  `json:"organization_id" format:"uuid"`
	SessionID            uuid.UUID  `json:"session_id" format:"uuid"`
	PreservationRevision int64      `json:"preservation_revision"`
	SourcePath           string     `json:"source_path"`
	Name                 string     `json:"name"`
	MIMEType             string     `json:"mime_type"`
	SizeBytes            int64      `json:"size_bytes"`
	SHA256               string     `json:"sha256"`
	CreatedAt            time.Time  `json:"created_at" format:"date-time"`
	ExpiresAt            *time.Time `json:"expires_at,omitempty" format:"date-time"`
	DownloadURL          string     `json:"download_url"`
}

// WorkspaceExecutionArtifactReadLimit bounds one retrieval response.
const WorkspaceExecutionArtifactReadLimit = 1024 * 1024

func workspaceExecutionArtifactPath(organization, session uuid.UUID) string {
	return fmt.Sprintf("/api/v2/organizations/%s/workspace-executions/%s/artifacts", organization, session)
}

// WorkspaceExecutionArtifacts lists preserved metadata, including expired results.
func (c *Client) WorkspaceExecutionArtifacts(ctx context.Context, organization, session uuid.UUID) ([]WorkspaceExecutionArtifact, error) {
	res, err := c.Request(ctx, http.MethodGet, workspaceExecutionArtifactPath(organization, session), nil)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, ReadBodyAsError(res)
	}
	var artifacts []WorkspaceExecutionArtifact
	err = ReadBodyAsJSON(res, &artifacts)
	return artifacts, err
}

// WorkspaceExecutionArtifactChunk contains a byte range and the complete artifact size.
type WorkspaceExecutionArtifactChunk struct {
	Content   []byte
	SizeBytes int64
}

// ReadWorkspaceExecutionArtifact retrieves an exact byte range. Empty files and
// offsets at the file size return an empty successful response.
func (c *Client) ReadWorkspaceExecutionArtifact(ctx context.Context, organization, session, artifact uuid.UUID, offset int64, limit int) (WorkspaceExecutionArtifactChunk, error) {
	var result WorkspaceExecutionArtifactChunk
	path := fmt.Sprintf("%s/%s?offset=%d&limit=%d", workspaceExecutionArtifactPath(organization, session), artifact, offset, limit)
	res, err := c.Request(ctx, http.MethodGet, path, nil)
	if err != nil {
		return result, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return result, ReadBodyAsError(res)
	}
	data, err := io.ReadAll(io.LimitReader(res.Body, WorkspaceExecutionArtifactReadLimit+1))
	if err != nil {
		return result, err
	}
	if len(data) > WorkspaceExecutionArtifactReadLimit {
		return result, xerrors.New("artifact response exceeds byte limit")
	}
	size, err := strconv.ParseInt(res.Header.Get("X-Artifact-Size"), 10, 64)
	if err != nil || size < 0 || offset < 0 || offset > size || int64(len(data)) != min(int64(limit), size-offset) {
		return result, xerrors.New("artifact response does not contain the requested byte range")
	}
	return WorkspaceExecutionArtifactChunk{Content: data, SizeBytes: size}, nil
}
