package agentapi

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"cdr.dev/slog"
	agentproto "github.com/coder/coder/v2/agent/proto"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/pubsub"
)

type MetadataAPI struct {
	AgentFn  func(context.Context) (database.WorkspaceAgent, error)
	Database database.Store
	Pubsub   pubsub.Pubsub
	Log      slog.Logger
}

func (a *MetadataAPI) BatchUpdateMetadata(ctx context.Context, req *agentproto.BatchUpdateMetadataRequest) (*agentproto.BatchUpdateMetadataResponse, error) {
	const (
		// maxValueLen is set to 2048 to stay under the 8000 byte Postgres
		// NOTIFY limit. Since both value and error can be set, the real payload
		// limit is 2 * 2048 * 4/3 <base64 expansion> = 5461 bytes + a few
		// hundred bytes for JSON syntax, key names, and metadata.
		maxValueLen = 2048
		maxErrorLen = maxValueLen
	)

	workspaceAgent, err := a.AgentFn(ctx)
	if err != nil {
		return nil, err
	}

	collectedAt := time.Now()
	dbUpdate := database.UpdateWorkspaceAgentMetadataParams{
		WorkspaceAgentID: workspaceAgent.ID,
		Key:              make([]string, 0, len(req.Metadata)),
		Value:            make([]string, 0, len(req.Metadata)),
		Error:            make([]string, 0, len(req.Metadata)),
		CollectedAt:      make([]time.Time, 0, len(req.Metadata)),
	}

	for _, md := range req.Metadata {
		metadataError := md.Result.Error

		// We overwrite the error if the provided payload is too long.
		if len(md.Result.Value) > maxValueLen {
			metadataError = fmt.Sprintf("value of %d bytes exceeded %d bytes", len(md.Result.Value), maxValueLen)
			md.Result.Value = md.Result.Value[:maxValueLen]
		}

		if len(md.Result.Error) > maxErrorLen {
			metadataError = fmt.Sprintf("error of %d bytes exceeded %d bytes", len(md.Result.Error), maxErrorLen)
			md.Result.Error = ""
		}

		// We don't want a misconfigured agent to fill the database.
		dbUpdate.Key = append(dbUpdate.Key, md.Key)
		dbUpdate.Value = append(dbUpdate.Value, md.Result.Value)
		dbUpdate.Error = append(dbUpdate.Error, metadataError)
		// We ignore the CollectedAt from the agent to avoid bugs caused by
		// clock skew.
		dbUpdate.CollectedAt = append(dbUpdate.CollectedAt, collectedAt)

		a.Log.Debug(
			ctx, "accepted metadata report",
			slog.F("collected_at", collectedAt),
			slog.F("original_collected_at", collectedAt),
			slog.F("key", md.Key),
			slog.F("value", ellipse(md.Result.Value, 16)),
		)
	}

	payload, err := json.Marshal(WorkspaceAgentMetadataChannelPayload{
		CollectedAt: collectedAt,
		Keys:        dbUpdate.Key,
	})
	if err != nil {
		return nil, xerrors.Errorf("marshal workspace agent metadata channel payload: %w", err)
	}

	err = a.Database.UpdateWorkspaceAgentMetadata(ctx, dbUpdate)
	if err != nil {
		return nil, xerrors.Errorf("update workspace agent metadata in database: %w", err)
	}

	err = a.Pubsub.Publish(WatchWorkspaceAgentMetadataChannel(workspaceAgent.ID), payload)
	if err != nil {
		return nil, xerrors.Errorf("publish workspace agent metadata: %w", err)
	}

	return &agentproto.BatchUpdateMetadataResponse{}, nil
}

func ellipse(v string, n int) string {
	if len(v) > n {
		return v[:n] + "..."
	}
	return v
}

type WorkspaceAgentMetadataChannelPayload struct {
	CollectedAt time.Time `json:"collected_at"`
	Keys        []string  `json:"keys"`
}

func WatchWorkspaceAgentMetadataChannel(id uuid.UUID) string {
	return "workspace_agent_metadata:" + id.String()
}
