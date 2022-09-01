package coderd

import (
	"encoding/json"
	"net/http"
	"reflect"
	"time"

	"github.com/google/uuid"
	"nhooyr.io/websocket"
	"nhooyr.io/websocket/wsjson"

	"cdr.dev/slog"
	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/coderd/httpapi"
	"github.com/coder/coder/coderd/httpmw"
	"github.com/coder/coder/codersdk"
)

func (api *API) workspaceAgentReportStats(rw http.ResponseWriter, r *http.Request) {
	api.websocketWaitMutex.Lock()
	api.websocketWaitGroup.Add(1)
	api.websocketWaitMutex.Unlock()
	defer api.websocketWaitGroup.Done()

	workspaceAgent := httpmw.WorkspaceAgent(r)
	resource, err := api.Database.GetWorkspaceResourceByID(r.Context(), workspaceAgent.ResourceID)
	if err != nil {
		httpapi.Write(rw, http.StatusBadRequest, codersdk.Response{
			Message: "Failed to get workspace resource.",
			Detail:  err.Error(),
		})
		return
	}

	build, err := api.Database.GetWorkspaceBuildByJobID(r.Context(), resource.JobID)
	if err != nil {
		httpapi.Write(rw, http.StatusBadRequest, codersdk.Response{
			Message: "Failed to get build.",
			Detail:  err.Error(),
		})
		return
	}

	workspace, err := api.Database.GetWorkspaceByID(r.Context(), build.WorkspaceID)
	if err != nil {
		httpapi.Write(rw, http.StatusBadRequest, codersdk.Response{
			Message: "Failed to get workspace.",
			Detail:  err.Error(),
		})
		return
	}

	conn, err := websocket.Accept(rw, r, &websocket.AcceptOptions{
		CompressionMode: websocket.CompressionDisabled,
	})
	if err != nil {
		httpapi.Write(rw, http.StatusBadRequest, codersdk.Response{
			Message: "Failed to accept websocket.",
			Detail:  err.Error(),
		})
		return
	}
	defer conn.Close(websocket.StatusAbnormalClosure, "")

	// Allow overriding the stat interval for debugging and testing purposes.
	ctx := r.Context()
	timer := time.NewTicker(api.AgentStatsRefreshInterval)
	var lastReport codersdk.AgentStatsReportResponse
	for {
		err := wsjson.Write(ctx, conn, codersdk.AgentStatsReportRequest{})
		if err != nil {
			httpapi.Write(rw, http.StatusBadRequest, codersdk.Response{
				Message: "Failed to write report request.",
				Detail:  err.Error(),
			})
			return
		}
		var rep codersdk.AgentStatsReportResponse

		err = wsjson.Read(ctx, conn, &rep)
		if err != nil {
			httpapi.Write(rw, http.StatusBadRequest, codersdk.Response{
				Message: "Failed to read report response.",
				Detail:  err.Error(),
			})
			return
		}

		repJSON, err := json.Marshal(rep)
		if err != nil {
			httpapi.Write(rw, http.StatusBadRequest, codersdk.Response{
				Message: "Failed to marshal stat json.",
				Detail:  err.Error(),
			})
			return
		}

		// Avoid inserting duplicate rows to preserve DB space.
		var insert = !reflect.DeepEqual(lastReport, rep)

		api.Logger.Debug(ctx, "read stats report",
			slog.F("interval", api.AgentStatsRefreshInterval),
			slog.F("agent", workspaceAgent.ID),
			slog.F("resource", resource.ID),
			slog.F("workspace", workspace.ID),
			slog.F("insert", insert),
			slog.F("payload", rep),
		)

		if insert {
			lastReport = rep

			_, err = api.Database.InsertAgentStat(ctx, database.InsertAgentStatParams{
				ID:          uuid.New(),
				CreatedAt:   time.Now(),
				AgentID:     workspaceAgent.ID,
				WorkspaceID: build.WorkspaceID,
				UserID:      workspace.OwnerID,
				TemplateID:  workspace.TemplateID,
				Payload:     json.RawMessage(repJSON),
			})
			if err != nil {
				httpapi.Write(rw, http.StatusBadRequest, codersdk.Response{
					Message: "Failed to insert agent stat.",
					Detail:  err.Error(),
				})
				return
			}
		}

		select {
		case <-timer.C:
			continue
		case <-ctx.Done():
			conn.Close(websocket.StatusNormalClosure, "")
			return
		}
	}
}
