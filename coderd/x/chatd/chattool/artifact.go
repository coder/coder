package chattool

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"path"
	"strings"

	"charm.land/fantasy"
	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/codersdk/workspacesdk"
)

const (
	// PublishArtifactToolName is the name of the publish_artifact tool.
	PublishArtifactToolName = "publish_artifact"
	// ReadArtifactToolName is the name of the read_artifact tool.
	ReadArtifactToolName = "read_artifact"

	// artifactWorkspaceDir is where read_artifact puts artifact copies.
	artifactWorkspaceDir = "/tmp/coder-artifacts"
)

const publishArtifactDescription = "Publish a workspace file as an artifact that the user can open, version and share by link. " +
	"Use it for reports, plans, diagrams and HTML pages that the user wants to look at. Use attach_file for a plain download. " +
	"Omit artifact_id to create an artifact. To add a version, pass artifact_id and the base_version from your last publish or read_artifact. " +
	"The user sees Markdown, HTML, SVG, Mermaid (.mmd), images, JSON, CSV and text rendered. " +
	"HTML runs in a sandbox with no network. Scripts load only from cdnjs, unpkg, jsDelivr /npm/, and the Tailwind and jQuery CDNs. Inline all other CSS, scripts and images. " +
	`To send a message to this chat from a click handler, call parent.postMessage({type: "coder:artifact-prompt", text}, "*"). ` +
	"The text is sent as a user message at once."

const readArtifactDescription = "Copy an artifact into the workspace so that you can change it. " +
	"When a message holds a link with /agents/artifacts/<id>, call this tool first. Then edit the file and publish it with that artifact_id."

// ErrArtifactNotFound is returned when the artifact does not exist or the
// chat cannot use it.
var ErrArtifactNotFound = xerrors.New("artifact not found")

// StaleBaseError reports that a publish named a base version that is not
// the latest version of the artifact.
type StaleBaseError struct {
	Latest int32
}

func (e *StaleBaseError) Error() string {
	return fmt.Sprintf("v%d is the latest version", e.Latest)
}

// PublishArtifactParams is one publish of a workspace file.
type PublishArtifactParams struct {
	// ArtifactID is unset to create an artifact.
	ArtifactID  uuid.NullUUID
	BaseVersion int32
	// Title is empty to keep the current title, or to default it on create.
	Title string
	Label string
	Name  string
	Data  []byte
}

// PublishedArtifact is the version that a publish created.
type PublishedArtifact struct {
	ArtifactID uuid.UUID
	Version    int32
	Title      string
	Name       string
}

// ArtifactContent is one version of an artifact with its bytes.
type ArtifactContent struct {
	ArtifactID uuid.UUID
	Version    int32
	Title      string
	Name       string
	Data       []byte
	// Untrusted is set when a user other than the chat owner published
	// this version.
	Untrusted bool
}

// ArtifactStore stores artifact versions for one chat. It applies the
// access rules of that chat.
type ArtifactStore interface {
	Publish(ctx context.Context, params PublishArtifactParams) (PublishedArtifact, error)
	// Read returns the given version, or the latest when version is 0.
	Read(ctx context.Context, artifactID uuid.UUID, version int32) (ArtifactContent, error)
}

// ArtifactOptions configures the artifact tools.
type ArtifactOptions struct {
	GetWorkspaceConn func(context.Context) (workspacesdk.AgentConn, error)
	Store            ArtifactStore
}

// PublishArtifactArgs are the arguments for the publish_artifact tool.
type PublishArtifactArgs struct {
	Path        string `json:"path" description:"Absolute path of the workspace file to publish."`
	ArtifactID  string `json:"artifact_id,omitempty" description:"The artifact to add a version to. Omit to create an artifact."`
	BaseVersion int32  `json:"base_version,omitempty" description:"The latest version you know of. Required with artifact_id."`
	Title       string `json:"title,omitempty" description:"Artifact title, at most 200 characters. Defaults to the file name without its extension on create."`
	Label       string `json:"label,omitempty" description:"Short note for this version, at most 60 characters."`
}

// ReadArtifactArgs are the arguments for the read_artifact tool.
type ReadArtifactArgs struct {
	ArtifactID string `json:"artifact_id" description:"The artifact ID, for example from a /agents/artifacts/<id> link."`
	Version    int32  `json:"version,omitempty" description:"The version to read. Omit for the latest."`
}

// ArtifactURL is the relative link to an artifact page.
func ArtifactURL(artifactID uuid.UUID) string {
	return "/agents/artifacts/" + artifactID.String()
}

// PublishArtifact stores a workspace file as the next version of an artifact.
func PublishArtifact(opts ArtifactOptions) fantasy.AgentTool {
	return fantasy.NewAgentTool(PublishArtifactToolName, publishArtifactDescription,
		func(ctx context.Context, args PublishArtifactArgs, _ fantasy.ToolCall) (fantasy.ToolResponse, error) {
			if opts.GetWorkspaceConn == nil || opts.Store == nil {
				return fantasy.NewTextErrorResponse("artifact storage is not configured"), nil
			}
			params := PublishArtifactParams{
				BaseVersion: args.BaseVersion,
				Title:       strings.TrimSpace(args.Title),
				Label:       strings.TrimSpace(args.Label),
			}
			if id := strings.TrimSpace(args.ArtifactID); id != "" {
				parsed, err := uuid.Parse(id)
				if err != nil {
					return fantasy.NewTextErrorResponse("artifact_id must be a UUID"), nil
				}
				params.ArtifactID = uuid.NullUUID{UUID: parsed, Valid: true}
			}
			if params.ArtifactID.Valid && params.BaseVersion <= 0 {
				return fantasy.NewTextErrorResponse("base_version is required with artifact_id. Call read_artifact to learn the latest version."), nil
			}
			if !params.ArtifactID.Valid && params.BaseVersion != 0 {
				return fantasy.NewTextErrorResponse("base_version needs artifact_id"), nil
			}
			if len([]rune(params.Label)) > 60 {
				return fantasy.NewTextErrorResponse("label must be at most 60 characters"), nil
			}
			conn, err := opts.GetWorkspaceConn(ctx)
			if err != nil {
				return fantasy.NewTextErrorResponse(err.Error()), nil
			}
			name, data, err := readWorkspaceFile(ctx, conn, args.Path)
			if err != nil {
				return fantasy.NewTextErrorResponse(err.Error()), nil
			}
			params.Name = name
			params.Data = data
			if !params.ArtifactID.Valid && params.Title == "" {
				params.Title = defaultArtifactTitle(name)
			}
			if len([]rune(params.Title)) > 200 {
				return fantasy.NewTextErrorResponse("title must be at most 200 characters"), nil
			}
			published, err := opts.Store.Publish(ctx, params)
			var stale *StaleBaseError
			if errors.As(err, &stale) {
				return fantasy.NewTextErrorResponse(fmt.Sprintf(
					"v%d is the latest. Call read_artifact, merge, then publish with base_version %d.",
					stale.Latest, stale.Latest)), nil
			}
			if err != nil {
				return fantasy.NewTextErrorResponse(err.Error()), nil
			}
			return toolResponse(map[string]any{
				"ok":          true,
				"artifact_id": published.ArtifactID.String(),
				"version":     published.Version,
				"title":       published.Title,
				"name":        published.Name,
				"url":         ArtifactURL(published.ArtifactID),
			}), nil
		})
}

// ReadArtifact copies an artifact version into the workspace.
func ReadArtifact(opts ArtifactOptions) fantasy.AgentTool {
	return fantasy.NewAgentTool(ReadArtifactToolName, readArtifactDescription,
		func(ctx context.Context, args ReadArtifactArgs, _ fantasy.ToolCall) (fantasy.ToolResponse, error) {
			if opts.GetWorkspaceConn == nil || opts.Store == nil {
				return fantasy.NewTextErrorResponse("artifact storage is not configured"), nil
			}
			artifactID, err := uuid.Parse(strings.TrimSpace(args.ArtifactID))
			if err != nil {
				return fantasy.NewTextErrorResponse("artifact_id must be a UUID"), nil
			}
			if args.Version < 0 {
				return fantasy.NewTextErrorResponse("version must be positive"), nil
			}
			content, err := opts.Store.Read(ctx, artifactID, args.Version)
			if err != nil {
				return fantasy.NewTextErrorResponse(err.Error()), nil
			}
			conn, err := opts.GetWorkspaceConn(ctx)
			if err != nil {
				return fantasy.NewTextErrorResponse(err.Error()), nil
			}
			target := artifactWorkspacePath(content.ArtifactID, content.Name)
			if err := conn.WriteFile(ctx, target, bytes.NewReader(content.Data)); err != nil {
				return fantasy.NewTextErrorResponse(err.Error()), nil
			}
			return toolResponse(map[string]any{
				"ok":          true,
				"artifact_id": content.ArtifactID.String(),
				"version":     content.Version,
				"title":       content.Title,
				"name":        content.Name,
				"path":        target,
				"untrusted":   content.Untrusted,
			}), nil
		})
}

// readWorkspaceFile reads a workspace file with the read and size limit of
// attach_file and returns its base name.
func readWorkspaceFile(ctx context.Context, conn workspacesdk.AgentConn, filePath string) (string, []byte, error) {
	filePath = strings.TrimSpace(filePath)
	if filePath == "" {
		return "", nil, xerrors.New("path is required (use an absolute path, e.g. /home/coder/report.md)")
	}
	if conn == nil {
		return "", nil, xerrors.New("workspace connection is not configured")
	}
	reader, _, err := conn.ReadFile(ctx, filePath, 0, maxAttachmentSize+1)
	if err != nil {
		return "", nil, err
	}
	defer reader.Close()
	data, err := io.ReadAll(io.LimitReader(reader, maxAttachmentSize+1))
	if err != nil {
		return "", nil, err
	}
	if len(data) == 0 {
		return "", nil, xerrors.New("file is empty")
	}
	if len(data) > maxAttachmentSize {
		return "", nil, xerrors.Errorf("file exceeds %d MiB size limit", maxAttachmentSize>>20)
	}
	trimmed := strings.TrimRight(filePath, "/\\")
	name := trimmed
	if idx := strings.LastIndexAny(trimmed, "/\\"); idx >= 0 {
		name = trimmed[idx+1:]
	}
	if name == "" {
		return "", nil, xerrors.New("path must name a file")
	}
	return name, data, nil
}

// defaultArtifactTitle is the file name without its extension.
func defaultArtifactTitle(name string) string {
	if title := strings.TrimSuffix(name, path.Ext(name)); title != "" {
		return title
	}
	return name
}

// artifactWorkspacePath is where read_artifact writes a version. The name
// is reduced to one path element so that it cannot leave the directory.
func artifactWorkspacePath(artifactID uuid.UUID, name string) string {
	name = strings.NewReplacer("/", "_", "\\", "_").Replace(name)
	if name == "" || name == "." || name == ".." {
		name = "artifact"
	}
	return path.Join(artifactWorkspaceDir, artifactID.String(), name)
}
