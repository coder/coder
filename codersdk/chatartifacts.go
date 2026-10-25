package codersdk

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/google/uuid"
)

// ChatArtifact is a versioned file that an agent published from a
// workspace. It outlives the chat and the workspace that made it. The
// owner sees it, and when ProjectID is set, the project members see it
// too.
type ChatArtifact struct {
	ID             uuid.UUID  `json:"id" format:"uuid"`
	OrganizationID uuid.UUID  `json:"organization_id" format:"uuid"`
	OwnerID        uuid.UUID  `json:"owner_id" format:"uuid"`
	OwnerUsername  string     `json:"owner_username"`
	ProjectID      *uuid.UUID `json:"project_id,omitempty" format:"uuid"`
	// ProjectName is empty when ProjectID is unset.
	ProjectName   string    `json:"project_name"`
	Title         string    `json:"title"`
	LatestVersion int32     `json:"latest_version"`
	CreatedAt     time.Time `json:"created_at" format:"date-time"`
	UpdatedAt     time.Time `json:"updated_at" format:"date-time"`
	// Versions is set only by GetChatArtifact, newest first.
	Versions []ChatArtifactVersion `json:"versions,omitempty"`
}

// ChatArtifactVersion describes one published version. The bytes come
// from ChatArtifactVersionData.
type ChatArtifactVersion struct {
	Version int32  `json:"version"`
	Label   string `json:"label"`
	// Name is the file name. Its extension picks the renderer.
	Name              string     `json:"name"`
	MediaType         string     `json:"media_type"`
	Size              int64      `json:"size"`
	CreatedBy         *uuid.UUID `json:"created_by,omitempty" format:"uuid"`
	CreatedByUsername string     `json:"created_by_username"`
	// ChatID is the chat that published this version. It is unset after
	// that chat is deleted.
	ChatID *uuid.UUID `json:"chat_id,omitempty" format:"uuid"`
	// ChatTitle is set only when the caller owns that chat.
	ChatTitle string    `json:"chat_title"`
	CreatedAt time.Time `json:"created_at" format:"date-time"`
}

func chatArtifactPath(artifactID uuid.UUID) string {
	return fmt.Sprintf("/api/experimental/chats/artifacts/%s", artifactID)
}

// ListChatArtifacts lists the artifacts that the user owns or can see
// through a chat project, across all organizations, newest update first.
func (c *ExperimentalClient) ListChatArtifacts(ctx context.Context) ([]ChatArtifact, error) {
	res, err := c.Request(ctx, http.MethodGet, "/api/experimental/chats/artifacts", nil)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, ReadBodyAsError(res)
	}
	var artifacts []ChatArtifact
	return artifacts, ReadBodyAsJSON(res, &artifacts)
}

// GetChatArtifact gets an artifact and the metadata of all its versions.
func (c *ExperimentalClient) GetChatArtifact(ctx context.Context, artifactID uuid.UUID) (ChatArtifact, error) {
	res, err := c.Request(ctx, http.MethodGet, chatArtifactPath(artifactID), nil)
	if err != nil {
		return ChatArtifact{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return ChatArtifact{}, ReadBodyAsError(res)
	}
	var artifact ChatArtifact
	return artifact, ReadBodyAsJSON(res, &artifact)
}

// ChatArtifactVersionData returns the bytes and the content type of one
// artifact version.
func (c *ExperimentalClient) ChatArtifactVersionData(ctx context.Context, artifactID uuid.UUID, version int32) ([]byte, string, error) {
	res, err := c.Request(ctx, http.MethodGet, fmt.Sprintf("%s/versions/%d", chatArtifactPath(artifactID), version), nil)
	if err != nil {
		return nil, "", err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, "", ReadBodyAsError(res)
	}
	data, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, "", err
	}
	return data, res.Header.Get("Content-Type"), nil
}
