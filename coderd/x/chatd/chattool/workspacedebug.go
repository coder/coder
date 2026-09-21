package chattool

import (
	"context"
	"strings"

	"charm.land/fantasy"
	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/x/workspacedebug"
)

// Read-only tools for workspace failure debugging chats. They run as the chat
// owner, so RBAC decides what the model may read, and they never dial an
// agent: a failed build usually has none.

const (
	// WorkspaceDebugLogPageLines bounds one page of logs returned by the
	// build and agent log tools.
	WorkspaceDebugLogPageLines = 400
	// WorkspaceDebugFileMaxRunes bounds a single template file returned by
	// get_template_version_files.
	WorkspaceDebugFileMaxRunes = 40000
)

// WorkspaceDebugOptions configures the workspace debugging tools.
type WorkspaceDebugOptions struct {
	OwnerID uuid.UUID
}

type getWorkspaceBuildLogsArgs struct {
	BuildID string `json:"build_id" description:"The workspace build ID (UUID) whose provisioner logs to read."`
	AfterID int64  `json:"after_id,omitempty" description:"Return logs with an ID greater than this value. Use the last returned id to page through long logs. Defaults to the beginning."`
}

// GetWorkspaceBuildLogs returns a tool that pages through the provisioner logs
// of a workspace build the chat owner can read.
func GetWorkspaceBuildLogs(db database.Store, options WorkspaceDebugOptions) fantasy.AgentTool {
	return fantasy.NewAgentTool(
		"get_workspace_build_logs",
		"Read the provisioner (Terraform) logs of a workspace build. Returns up "+
			"to 400 lines per call with their ids; pass after_id to continue. Use "+
			"this to see the complete output around a build failure.",
		func(ctx context.Context, args getWorkspaceBuildLogsArgs, _ fantasy.ToolCall) (fantasy.ToolResponse, error) {
			buildID, err := uuid.Parse(strings.TrimSpace(args.BuildID))
			if err != nil {
				return fantasy.NewTextErrorResponse(xerrors.Errorf("invalid build_id: %w", err).Error()), nil
			}
			ctx, err = asOwner(ctx, db, options.OwnerID)
			if err != nil {
				return fantasy.NewTextErrorResponse(err.Error()), nil
			}
			build, err := db.GetWorkspaceBuildByID(ctx, buildID)
			if err != nil {
				return fantasy.NewTextErrorResponse("workspace build not found"), nil
			}
			job, err := db.GetProvisionerJobByID(ctx, build.JobID)
			if err != nil {
				return fantasy.NewTextErrorResponse("workspace build job not found"), nil
			}
			after := args.AfterID
			if after <= 0 {
				after = -1
			}
			logs, err := db.GetProvisionerLogsAfterID(ctx, database.GetProvisionerLogsAfterIDParams{
				JobID:        job.ID,
				CreatedAfter: after,
			})
			if err != nil {
				return fantasy.NewTextErrorResponse(xerrors.Errorf("read build logs: %w", err).Error()), nil
			}
			hasMore := false
			if len(logs) > WorkspaceDebugLogPageLines {
				logs = logs[:WorkspaceDebugLogPageLines]
				hasMore = true
			}
			lines := make([]map[string]any, 0, len(logs))
			for _, l := range logs {
				lines = append(lines, map[string]any{
					"id":     l.ID,
					"stage":  l.Stage,
					"level":  string(l.Level),
					"output": l.Output,
				})
			}
			result := map[string]any{
				"build_id":     build.ID.String(),
				"build_number": build.BuildNumber,
				"transition":   string(build.Transition),
				"job_status":   string(job.JobStatus),
				"logs":         lines,
				"has_more":     hasMore,
			}
			if job.Error.Valid && job.Error.String != "" {
				result["job_error"] = job.Error.String
			}
			if job.ErrorCode.Valid && job.ErrorCode.String != "" {
				result["job_error_code"] = job.ErrorCode.String
			}
			if hasMore && len(logs) > 0 {
				result["next_after_id"] = logs[len(logs)-1].ID
			}
			return toolResponse(result), nil
		},
	)
}

type getWorkspaceAgentLogsArgs struct {
	AgentID string `json:"agent_id" description:"The workspace agent ID (UUID) whose startup logs to read."`
	AfterID int64  `json:"after_id,omitempty" description:"Return logs with an ID greater than this value. Defaults to the beginning."`
}

// GetWorkspaceAgentLogs returns a tool that reads the startup script logs a
// workspace agent has already reported to coderd. It does not connect to the
// agent.
func GetWorkspaceAgentLogs(db database.Store, options WorkspaceDebugOptions) fantasy.AgentTool {
	return fantasy.NewAgentTool(
		"get_workspace_agent_logs",
		"Read the startup script logs a workspace agent reported to Coder, "+
			"along with its lifecycle state. Use this when a build succeeded but "+
			"the agent ended in start_error or start_timeout.",
		func(ctx context.Context, args getWorkspaceAgentLogsArgs, _ fantasy.ToolCall) (fantasy.ToolResponse, error) {
			agentID, err := uuid.Parse(strings.TrimSpace(args.AgentID))
			if err != nil {
				return fantasy.NewTextErrorResponse(xerrors.Errorf("invalid agent_id: %w", err).Error()), nil
			}
			ctx, err = asOwner(ctx, db, options.OwnerID)
			if err != nil {
				return fantasy.NewTextErrorResponse(err.Error()), nil
			}
			agent, err := db.GetWorkspaceAgentByID(ctx, agentID)
			if err != nil {
				return fantasy.NewTextErrorResponse("workspace agent not found"), nil
			}
			logs, err := db.GetWorkspaceAgentLogsAfter(ctx, database.GetWorkspaceAgentLogsAfterParams{
				AgentID:      agent.ID,
				CreatedAfter: args.AfterID,
			})
			if err != nil {
				return fantasy.NewTextErrorResponse(xerrors.Errorf("read agent logs: %w", err).Error()), nil
			}
			hasMore := false
			if len(logs) > WorkspaceDebugLogPageLines {
				logs = logs[:WorkspaceDebugLogPageLines]
				hasMore = true
			}
			lines := make([]map[string]any, 0, len(logs))
			for _, l := range logs {
				lines = append(lines, map[string]any{
					"id":     l.ID,
					"level":  string(l.Level),
					"output": l.Output,
				})
			}
			result := map[string]any{
				"agent_id":        agent.ID.String(),
				"name":            agent.Name,
				"lifecycle_state": string(agent.LifecycleState),
				"logs":            lines,
				"has_more":        hasMore,
			}
			if agent.TroubleshootingURL != "" {
				result["troubleshooting_url"] = agent.TroubleshootingURL
			}
			if hasMore && len(logs) > 0 {
				result["next_after_id"] = logs[len(logs)-1].ID
			}
			return toolResponse(result), nil
		},
	)
}

type getTemplateVersionFilesArgs struct {
	TemplateVersionID string `json:"template_version_id" description:"The template version ID (UUID) recorded on the failed build."`
	Path              string `json:"path,omitempty" description:"Path of a file inside the template to read. Omit to list the Terraform files in the version."`
}

// GetTemplateVersionFiles returns a tool that lists or reads Terraform files
// from the exact template version a build used. Access follows the owner's
// permission on the template file, so members of templates whose source is
// restricted get an error they can relay as "ask a template admin".
func GetTemplateVersionFiles(db database.Store, options WorkspaceDebugOptions) fantasy.AgentTool {
	return fantasy.NewAgentTool(
		"get_template_version_files",
		"List the Terraform files in a template version, or read one file by "+
			"path. Use the template_version_id from the failure context so you "+
			"inspect the exact source the failed build ran.",
		func(ctx context.Context, args getTemplateVersionFilesArgs, _ fantasy.ToolCall) (fantasy.ToolResponse, error) {
			versionID, err := uuid.Parse(strings.TrimSpace(args.TemplateVersionID))
			if err != nil {
				return fantasy.NewTextErrorResponse(xerrors.Errorf("invalid template_version_id: %w", err).Error()), nil
			}
			ctx, err = asOwner(ctx, db, options.OwnerID)
			if err != nil {
				return fantasy.NewTextErrorResponse(err.Error()), nil
			}
			version, err := db.GetTemplateVersionByID(ctx, versionID)
			if err != nil {
				return fantasy.NewTextErrorResponse("template version not found"), nil
			}
			job, err := db.GetProvisionerJobByID(ctx, version.JobID)
			if err != nil {
				return fantasy.NewTextErrorResponse("template version job not found"), nil
			}
			file, err := db.GetFileByID(ctx, job.FileID)
			if err != nil {
				return fantasy.NewTextErrorResponse(
					"you are not permitted to read this template's source; " +
						"changes to the template must be made by a template administrator",
				), nil
			}
			// Read without a byte cap here; the response is bounded per file
			// below and listing needs every path.
			files, _, err := workspacedebug.ReadTerraformFiles(file.Data, 1<<30)
			if err != nil {
				return fantasy.NewTextErrorResponse(xerrors.Errorf("read template archive: %w", err).Error()), nil
			}
			wanted := strings.TrimPrefix(strings.TrimSpace(args.Path), "./")
			if wanted == "" {
				paths := make([]map[string]any, 0, len(files))
				for _, f := range files {
					paths = append(paths, map[string]any{
						"path":  f.Path,
						"bytes": len(f.Content),
					})
				}
				return toolResponse(map[string]any{
					"template_version_id": version.ID.String(),
					"name":                version.Name,
					"files":               paths,
				}), nil
			}
			for _, f := range files {
				if f.Path != wanted {
					continue
				}
				content := f.Content
				truncated := false
				if t := truncateRunes(content, WorkspaceDebugFileMaxRunes); len(t) < len(content) {
					content = t
					truncated = true
				}
				return toolResponse(map[string]any{
					"template_version_id": version.ID.String(),
					"path":                f.Path,
					"content":             content,
					"truncated":           truncated,
				}), nil
			}
			return fantasy.NewTextErrorResponse("file not found in template version: " + wanted), nil
		},
	)
}
