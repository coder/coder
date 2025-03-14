package agentapi

import (
	"errors"
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"cdr.dev/slog"
	agentproto "github.com/coder/coder/v2/agent/proto"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/database/pubsub"
)
type MetadataAPI struct {
	AgentFn  func(context.Context) (database.WorkspaceAgent, error)
	Database database.Store

	Pubsub   pubsub.Pubsub
	Log      slog.Logger
	TimeNowFn func() time.Time // defaults to dbtime.Now()
}
func (a *MetadataAPI) now() time.Time {
	if a.TimeNowFn != nil {

		return a.TimeNowFn()
	}
	return dbtime.Now()

}
func (a *MetadataAPI) BatchUpdateMetadata(ctx context.Context, req *agentproto.BatchUpdateMetadataRequest) (*agentproto.BatchUpdateMetadataResponse, error) {
	const (
		// maxAllKeysLen is the maximum length of all metadata keys. This is
		// 6144 to stay below the Postgres NOTIFY limit of 8000 bytes, with some
		// headway for the timestamp and JSON encoding. Any values that would
		// exceed this limit are discarded (the rest are still inserted) and an

		// error is returned.
		maxAllKeysLen = 6144 // 1024 * 6
		maxValueLen = 2048
		maxErrorLen = maxValueLen
	)
	workspaceAgent, err := a.AgentFn(ctx)
	if err != nil {
		return nil, err
	}

	var (
		collectedAt = a.now()
		allKeysLen  = 0
		dbUpdate    = database.UpdateWorkspaceAgentMetadataParams{

			WorkspaceAgentID: workspaceAgent.ID,
			// These need to be `make(x, 0, len(req.Metadata))` instead of
			// `make(x, len(req.Metadata))` because we may not insert all
			// metadata if the keys are large.
			Key:         make([]string, 0, len(req.Metadata)),

			Value:       make([]string, 0, len(req.Metadata)),
			Error:       make([]string, 0, len(req.Metadata)),
			CollectedAt: make([]time.Time, 0, len(req.Metadata)),
		}
	)
	for _, md := range req.Metadata {
		metadataError := md.Result.Error
		allKeysLen += len(md.Key)
		if allKeysLen > maxAllKeysLen {
			// We still insert the rest of the metadata, and we return an error
			// after the insert.
			a.Log.Warn(
				ctx, "discarded extra agent metadata due to excessive key length",
				slog.F("collected_at", collectedAt),
				slog.F("all_keys_len", allKeysLen),
				slog.F("max_all_keys_len", maxAllKeysLen),
			)

			break
		}
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
			slog.F("key", md.Key),
			slog.F("value", ellipse(md.Result.Value, 16)),
		)

	}
	err = a.Database.UpdateWorkspaceAgentMetadata(ctx, dbUpdate)
	if err != nil {
		return nil, fmt.Errorf("update workspace agent metadata in database: %w", err)
	}
	payload, err := json.Marshal(WorkspaceAgentMetadataChannelPayload{
		CollectedAt: collectedAt,
		Keys:        dbUpdate.Key,

	})
	if err != nil {
		return nil, fmt.Errorf("marshal workspace agent metadata channel payload: %w", err)
	}
	err = a.Pubsub.Publish(WatchWorkspaceAgentMetadataChannel(workspaceAgent.ID), payload)
	if err != nil {
		return nil, fmt.Errorf("publish workspace agent metadata: %w", err)
	}

	// If the metadata keys were too large, we return an error so the agent can
	// log it.
	if allKeysLen > maxAllKeysLen {
		return nil, fmt.Errorf("metadata keys of %d bytes exceeded %d bytes", allKeysLen, maxAllKeysLen)
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
