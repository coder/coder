package agentapi

import (
	"context"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"cdr.dev/slog"
	agentproto "github.com/coder/coder/v2/agent/proto"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/codersdk/agentsdk"
)

type LogsAPI struct {
	AgentFn                           func(context.Context) (database.WorkspaceAgent, error)
	Database                          database.Store
	Log                               slog.Logger
	PublishWorkspaceUpdateFn          func(context.Context, *database.WorkspaceAgent) error
	PublishWorkspaceAgentLogsUpdateFn func(ctx context.Context, workspaceAgentID uuid.UUID, msg agentsdk.LogsNotifyMessage)
}

func (a *LogsAPI) BatchCreateLogs(ctx context.Context, req *agentproto.BatchCreateLogsRequest) (*agentproto.BatchCreateLogsResponse, error) {
	workspaceAgent, err := a.AgentFn(ctx)
	if err != nil {
		return nil, err
	}

	if len(req.Logs) == 0 {
		return &agentproto.BatchCreateLogsResponse{}, nil
	}
	logSourceID, err := uuid.FromBytes(req.LogSourceId)
	if err != nil {
		return nil, xerrors.Errorf("parse log source ID %q: %w", req.LogSourceId, err)
	}

	// This is to support the legacy API where the log source ID was
	// not provided in the request body. We default to the external
	// log source in this case.
	if logSourceID == uuid.Nil {
		// Use the external log source
		externalSources, err := a.Database.InsertWorkspaceAgentLogSources(ctx, database.InsertWorkspaceAgentLogSourcesParams{
			WorkspaceAgentID: workspaceAgent.ID,
			CreatedAt:        dbtime.Now(),
			ID:               []uuid.UUID{agentsdk.ExternalLogSourceID},
			DisplayName:      []string{"External"},
			Icon:             []string{"/emojis/1f310.png"},
		})
		if database.IsUniqueViolation(err, database.UniqueWorkspaceAgentLogSourcesPkey) {
			err = nil
			logSourceID = agentsdk.ExternalLogSourceID
		}
		if err != nil {
			return nil, xerrors.Errorf("insert external workspace agent log source: %w", err)
		}
		if len(externalSources) == 1 {
			logSourceID = externalSources[0].ID
		}
	}

	output := make([]string, 0)
	level := make([]database.LogLevel, 0)
	outputLength := 0
	for _, logEntry := range req.Logs {
		output = append(output, logEntry.Output)
		outputLength += len(logEntry.Output)

		var dbLevel database.LogLevel
		switch logEntry.Level {
		case agentproto.Log_TRACE:
			dbLevel = database.LogLevelTrace
		case agentproto.Log_DEBUG:
			dbLevel = database.LogLevelDebug
		case agentproto.Log_INFO:
			dbLevel = database.LogLevelInfo
		case agentproto.Log_WARN:
			dbLevel = database.LogLevelWarn
		case agentproto.Log_ERROR:
			dbLevel = database.LogLevelError
		default:
			// Default to "info" to support older clients that didn't have the
			// level field.
			dbLevel = database.LogLevelInfo
		}
		level = append(level, dbLevel)
	}

	logs, err := a.Database.InsertWorkspaceAgentLogs(ctx, database.InsertWorkspaceAgentLogsParams{
		AgentID:      workspaceAgent.ID,
		CreatedAt:    dbtime.Now(),
		Output:       output,
		Level:        level,
		LogSourceID:  logSourceID,
		OutputLength: int32(outputLength),
	})
	if err != nil {
		if !database.IsWorkspaceAgentLogsLimitError(err) {
			return nil, xerrors.Errorf("insert workspace agent logs: %w", err)
		}
		if workspaceAgent.LogsOverflowed {
			return nil, xerrors.New("workspace agent logs overflowed")
		}
		err := a.Database.UpdateWorkspaceAgentLogOverflowByID(ctx, database.UpdateWorkspaceAgentLogOverflowByIDParams{
			ID:             workspaceAgent.ID,
			LogsOverflowed: true,
		})
		if err != nil {
			// We don't want to return here, because the agent will retry on
			// failure and this isn't a huge deal. The overflow state is just a
			// hint to the user that the logs are incomplete.
			a.Log.Warn(ctx, "failed to update workspace agent log overflow", slog.Error(err))
		}

		err = a.PublishWorkspaceUpdateFn(ctx, &workspaceAgent)
		if err != nil {
			return nil, xerrors.Errorf("publish workspace update: %w", err)
		}
		return nil, xerrors.New("workspace agent log limit exceeded")
	}

	// Publish by the lowest log ID inserted so the log stream will fetch
	// everything from that point.
	lowestLogID := logs[0].ID
	a.PublishWorkspaceAgentLogsUpdateFn(ctx, workspaceAgent.ID, agentsdk.LogsNotifyMessage{
		CreatedAfter: lowestLogID - 1,
	})

	if workspaceAgent.LogsLength == 0 {
		// If these are the first logs being appended, we publish a UI update
		// to notify the UI that logs are now available.
		err = a.PublishWorkspaceUpdateFn(ctx, &workspaceAgent)
		if err != nil {
			return nil, xerrors.Errorf("publish workspace update: %w", err)
		}
	}

	return &agentproto.BatchCreateLogsResponse{}, nil
}
