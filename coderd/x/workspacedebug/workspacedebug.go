// Package workspacedebug assembles a bounded, RBAC-scoped description of a
// failed workspace build for an AI debugging chat. It is a server-side
// counterpart to the workspace section of a support bundle: it pins the exact
// template version the failed build used, tails the provisioner and agent
// startup logs, and records what the requester is allowed to see so the model
// can decide whether the user can fix the problem or needs an administrator.
package workspacedebug

import (
	"archive/tar"
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"path"
	"sort"
	"strings"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
)

// Chat labels that mark a chat as a workspace debugging session. Chatd uses
// the kind label to attach the read-only debugging tools.
const (
	LabelKind        = "coder.workspace_debug"
	LabelKindValue   = "true"
	LabelWorkspaceID = "coder.workspace_debug/workspace_id"
	LabelBuildID     = "coder.workspace_debug/build_id"
	// LabelFailure stores the one-line failure summary the chat was opened
	// with so later lookups do not recollect the bundle.
	LabelFailure = "coder.workspace_debug/failure"
)

// IsDebugChat reports whether a chat's labels mark it as a workspace
// debugging session.
func IsDebugChat(labels map[string]string) bool {
	return labels[LabelKind] == LabelKindValue
}

const (
	// BuildLogTailLines is the number of trailing provisioner log lines
	// included verbatim. Failures almost always sit at the end of the log.
	BuildLogTailLines = 150
	// BuildLogErrorLines caps the number of earlier lines that look like
	// errors and are surfaced ahead of the tail.
	BuildLogErrorLines = 40
	// AgentLogTailLines is the per-agent startup log tail.
	AgentLogTailLines = 80
	// TemplateSourceMaxBytes caps the Terraform source included in the
	// bundle. The get_template_version_files tool serves the rest.
	TemplateSourceMaxBytes = 24 * 1024
	// MaxPromptBytes caps the rendered bundle so it cannot crowd out the
	// conversation in small context windows.
	MaxPromptBytes = 64 * 1024
)

// LogLine is a single log line with the fields the model needs.
type LogLine struct {
	ID     int64
	Stage  string
	Level  string
	Output string
}

// AgentInfo describes one agent of the failed build.
type AgentInfo struct {
	ID                 uuid.UUID
	Name               string
	LifecycleState     database.WorkspaceAgentLifecycleState
	TroubleshootingURL string
	Logs               []LogLine
	LogsTruncated      bool
}

// SourceFile is one template file included in the bundle.
type SourceFile struct {
	Path      string
	Content   string
	Truncated bool
}

// Bundle is the collected failure context.
type Bundle struct {
	Workspace     database.Workspace
	OwnerUsername string
	Build         database.WorkspaceBuild
	Job           database.ProvisionerJob

	Template        database.Template
	TemplateVersion database.TemplateVersion
	// ActiveVersionDiffers is true when the template has moved on from the
	// version this build used, which often means the fix is "update".
	ActiveVersionDiffers bool

	Parameters []database.WorkspaceBuildParameter

	BuildLogErrors []LogLine
	BuildLogTail   []LogLine
	BuildLogTotal  int

	Agents []AgentInfo

	// SourceFiles holds the template source when the requester may read it.
	SourceFiles []SourceFile
	// SourceAccessDenied is true when the requester lacks permission to read
	// the template source. It is a strong signal that admin help is needed.
	SourceAccessDenied bool
	SourceTruncated    bool

	// RequesterRoles are the RBAC role names of the requesting user.
	RequesterRoles []string
}

// Collect gathers the failure context for a workspace build. It must be called
// with a context carrying the requesting user's dbauthz actor so that every
// read is authorized as that user.
func Collect(ctx context.Context, db database.Store, buildID uuid.UUID) (Bundle, error) {
	var b Bundle
	var err error

	b.Build, err = db.GetWorkspaceBuildByID(ctx, buildID)
	if err != nil {
		return Bundle{}, xerrors.Errorf("get workspace build: %w", err)
	}
	b.Job, err = db.GetProvisionerJobByID(ctx, b.Build.JobID)
	if err != nil {
		return Bundle{}, xerrors.Errorf("get provisioner job: %w", err)
	}
	b.Workspace, err = db.GetWorkspaceByID(ctx, b.Build.WorkspaceID)
	if err != nil {
		return Bundle{}, xerrors.Errorf("get workspace: %w", err)
	}
	if owner, err := db.GetUserByID(ctx, b.Workspace.OwnerID); err == nil {
		b.OwnerUsername = owner.Username
	}
	b.Template, err = db.GetTemplateByID(ctx, b.Workspace.TemplateID)
	if err != nil {
		return Bundle{}, xerrors.Errorf("get template: %w", err)
	}
	b.TemplateVersion, err = db.GetTemplateVersionByID(ctx, b.Build.TemplateVersionID)
	if err != nil {
		return Bundle{}, xerrors.Errorf("get template version: %w", err)
	}
	b.ActiveVersionDiffers = b.Template.ActiveVersionID != b.Build.TemplateVersionID

	b.Parameters, err = db.GetWorkspaceBuildParameters(ctx, b.Build.ID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return Bundle{}, xerrors.Errorf("get build parameters: %w", err)
	}

	logs, err := db.GetProvisionerLogsAfterID(ctx, database.GetProvisionerLogsAfterIDParams{
		JobID:        b.Job.ID,
		CreatedAfter: -1,
	})
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return Bundle{}, xerrors.Errorf("get build logs: %w", err)
	}
	b.BuildLogTotal = len(logs)
	b.BuildLogErrors, b.BuildLogTail = splitBuildLogs(logs)

	b.Agents, err = collectAgents(ctx, db, b.Job.ID)
	if err != nil {
		return Bundle{}, err
	}

	b.SourceFiles, b.SourceTruncated, b.SourceAccessDenied, err = collectTemplateSource(ctx, db, b.TemplateVersion)
	if err != nil {
		return Bundle{}, err
	}

	if actor, ok := dbauthz.ActorFromContext(ctx); ok {
		for _, role := range actor.Roles.Names() {
			b.RequesterRoles = append(b.RequesterRoles, role.String())
		}
		sort.Strings(b.RequesterRoles)
	}

	return b, nil
}

// FailureSummary is the one-line error the user saw, suitable for filling
// into the opening prompt.
func (b Bundle) FailureSummary() string {
	if b.Job.Error.Valid && strings.TrimSpace(b.Job.Error.String) != "" {
		return firstLine(b.Job.Error.String)
	}
	for _, agent := range b.Agents {
		switch agent.LifecycleState {
		case database.WorkspaceAgentLifecycleStateStartError:
			return fmt.Sprintf("agent %q startup script failed", agent.Name)
		case database.WorkspaceAgentLifecycleStateStartTimeout:
			return fmt.Sprintf("agent %q startup script timed out", agent.Name)
		case database.WorkspaceAgentLifecycleStateShutdownError:
			return fmt.Sprintf("agent %q shutdown script failed", agent.Name)
		}
	}
	if b.Job.JobStatus == database.ProvisionerJobStatusCanceled {
		return "build was canceled"
	}
	return fmt.Sprintf("build %s finished with status %s", b.Build.Transition, b.Job.JobStatus)
}

// Prompt renders the bundle as markdown for injection into a chat system
// message. Output is bounded by MaxPromptBytes.
func (b Bundle) Prompt() string {
	var sb strings.Builder

	_, _ = fmt.Fprintf(&sb, "# Workspace failure context\n\n")
	_, _ = fmt.Fprintf(&sb, "## Workspace\n\n")
	_, _ = fmt.Fprintf(&sb, "- name: %s\n- owner: %s\n- workspace_id: %s\n", b.Workspace.Name, b.OwnerUsername, b.Workspace.ID)
	_, _ = fmt.Fprintf(&sb, "- build_id: %s\n- build_number: %d\n- transition: %s\n- reason: %s\n",
		b.Build.ID, b.Build.BuildNumber, b.Build.Transition, b.Build.Reason)
	_, _ = fmt.Fprintf(&sb, "- job_status: %s\n", b.Job.JobStatus)
	if b.Job.ErrorCode.Valid && b.Job.ErrorCode.String != "" {
		_, _ = fmt.Fprintf(&sb, "- job_error_code: %s\n", b.Job.ErrorCode.String)
	}
	if b.Job.Error.Valid && b.Job.Error.String != "" {
		_, _ = fmt.Fprintf(&sb, "- job_error: |\n%s\n", indent(strings.TrimSpace(b.Job.Error.String), "    "))
	}

	_, _ = fmt.Fprintf(&sb, "\n## Template\n\n")
	_, _ = fmt.Fprintf(&sb, "- template: %s (template_id %s)\n", b.Template.Name, b.Template.ID)
	_, _ = fmt.Fprintf(&sb, "- template_version: %s (template_version_id %s)\n", b.TemplateVersion.Name, b.TemplateVersion.ID)
	if b.ActiveVersionDiffers {
		_, _ = fmt.Fprintf(&sb, "- note: the template's active version (%s) differs from the version this build used. Updating the workspace may pick up a fix.\n", b.Template.ActiveVersionID)
	}
	if b.Template.Deprecated != "" {
		_, _ = fmt.Fprintf(&sb, "- deprecated: %s\n", b.Template.Deprecated)
	}

	if len(b.Parameters) > 0 {
		_, _ = fmt.Fprintf(&sb, "\n## Build parameters\n\n")
		for _, p := range b.Parameters {
			_, _ = fmt.Fprintf(&sb, "- %s = %s\n", p.Name, redactParameter(p.Name, p.Value))
		}
	}

	_, _ = fmt.Fprintf(&sb, "\n## Provisioner build logs (%d lines total)\n\n", b.BuildLogTotal)
	if len(b.BuildLogErrors) > 0 {
		_, _ = fmt.Fprintf(&sb, "Lines that look like errors, in order:\n\n```\n")
		writeLogLines(&sb, b.BuildLogErrors)
		_, _ = fmt.Fprintf(&sb, "```\n\n")
	}
	if len(b.BuildLogTail) > 0 {
		_, _ = fmt.Fprintf(&sb, "Last %d lines:\n\n```\n", len(b.BuildLogTail))
		writeLogLines(&sb, b.BuildLogTail)
		_, _ = fmt.Fprintf(&sb, "```\n")
	} else {
		_, _ = fmt.Fprintf(&sb, "No provisioner logs were recorded.\n")
	}
	if b.BuildLogTotal > len(b.BuildLogTail) {
		_, _ = fmt.Fprintf(&sb, "\nUse get_workspace_build_logs with build_id %s to read the full log.\n", b.Build.ID)
	}

	_, _ = fmt.Fprintf(&sb, "\n## Agents\n\n")
	if len(b.Agents) == 0 {
		_, _ = fmt.Fprintf(&sb, "No agents exist for this build: provisioning failed before any resources were created.\n")
	}
	for _, agent := range b.Agents {
		_, _ = fmt.Fprintf(&sb, "### %s (agent_id %s)\n\n- lifecycle_state: %s\n", agent.Name, agent.ID, agent.LifecycleState)
		if agent.TroubleshootingURL != "" {
			_, _ = fmt.Fprintf(&sb, "- troubleshooting_url: %s\n", agent.TroubleshootingURL)
		}
		if len(agent.Logs) > 0 {
			_, _ = fmt.Fprintf(&sb, "\nStartup log tail:\n\n```\n")
			writeLogLines(&sb, agent.Logs)
			_, _ = fmt.Fprintf(&sb, "```\n")
			if agent.LogsTruncated {
				_, _ = fmt.Fprintf(&sb, "\nUse get_workspace_agent_logs with agent_id %s for the full log.\n", agent.ID)
			}
		}
		_, _ = sb.WriteString("\n")
	}

	_, _ = fmt.Fprintf(&sb, "## Template source\n\n")
	switch {
	case b.SourceAccessDenied:
		_, _ = fmt.Fprintf(&sb, "The requesting user is not permitted to read this template's source. Fixes that require editing the template must go through a template administrator.\n")
	case len(b.SourceFiles) == 0:
		_, _ = fmt.Fprintf(&sb, "No Terraform files were found in the template version.\n")
	default:
		for _, f := range b.SourceFiles {
			_, _ = fmt.Fprintf(&sb, "### %s\n\n```hcl\n%s\n```\n", f.Path, strings.TrimRight(f.Content, "\n"))
			if f.Truncated {
				_, _ = fmt.Fprintf(&sb, "\n(truncated)\n")
			}
			_, _ = sb.WriteString("\n")
		}
		if b.SourceTruncated {
			_, _ = fmt.Fprintf(&sb, "Not all files are shown. Use get_template_version_files with template_version_id %s to list and read the rest.\n", b.TemplateVersion.ID)
		}
	}

	_, _ = fmt.Fprintf(&sb, "\n## Requesting user\n\n")
	if len(b.RequesterRoles) == 0 {
		_, _ = fmt.Fprintf(&sb, "- roles: unknown\n")
	} else {
		_, _ = fmt.Fprintf(&sb, "- roles: %s\n", strings.Join(b.RequesterRoles, ", "))
	}
	_, _ = fmt.Fprintf(&sb, "- can_edit_template: %t\n", !b.SourceAccessDenied && b.canEditTemplate())

	out := sb.String()
	if len(out) > MaxPromptBytes {
		out = out[:MaxPromptBytes] + "\n\n(context truncated)\n"
	}
	return out
}

// canEditTemplate is a coarse role-name check. It intentionally errs toward
// false so the model does not tell a member to edit Terraform they cannot
// push.
func (b Bundle) canEditTemplate() bool {
	for _, role := range b.RequesterRoles {
		if role == "owner" || strings.HasPrefix(role, "template-admin") || strings.HasPrefix(role, "organization-template-admin") || strings.HasPrefix(role, "organization-admin") {
			return true
		}
	}
	return false
}

func collectAgents(ctx context.Context, db database.Store, jobID uuid.UUID) ([]AgentInfo, error) {
	resources, err := db.GetWorkspaceResourcesByJobID(ctx, jobID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, xerrors.Errorf("get workspace resources: %w", err)
	}
	if len(resources) == 0 {
		return nil, nil
	}
	resourceIDs := make([]uuid.UUID, 0, len(resources))
	for _, r := range resources {
		resourceIDs = append(resourceIDs, r.ID)
	}
	agents, err := db.GetWorkspaceAgentsByResourceIDs(ctx, resourceIDs)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, xerrors.Errorf("get workspace agents: %w", err)
	}
	infos := make([]AgentInfo, 0, len(agents))
	for _, agent := range agents {
		info := AgentInfo{
			ID:                 agent.ID,
			Name:               agent.Name,
			LifecycleState:     agent.LifecycleState,
			TroubleshootingURL: agent.TroubleshootingURL,
		}
		logs, err := db.GetWorkspaceAgentLogsAfter(ctx, database.GetWorkspaceAgentLogsAfterParams{
			AgentID:      agent.ID,
			CreatedAfter: 0,
		})
		if err != nil && !errors.Is(err, sql.ErrNoRows) && !dbauthz.IsNotAuthorizedError(err) {
			return nil, xerrors.Errorf("get agent logs: %w", err)
		}
		if len(logs) > AgentLogTailLines {
			logs = logs[len(logs)-AgentLogTailLines:]
			info.LogsTruncated = true
		}
		for _, l := range logs {
			info.Logs = append(info.Logs, LogLine{ID: l.ID, Level: string(l.Level), Output: l.Output})
		}
		infos = append(infos, info)
	}
	return infos, nil
}

// collectTemplateSource reads Terraform files out of the template version's
// tar. A not-authorized error is reported through the accessDenied flag rather
// than failing the whole bundle.
func collectTemplateSource(ctx context.Context, db database.Store, version database.TemplateVersion) (files []SourceFile, truncated bool, accessDenied bool, err error) {
	job, err := db.GetProvisionerJobByID(ctx, version.JobID)
	if err != nil {
		if dbauthz.IsNotAuthorizedError(err) {
			return nil, false, true, nil
		}
		return nil, false, false, xerrors.Errorf("get template version job: %w", err)
	}
	file, err := db.GetFileByID(ctx, job.FileID)
	if err != nil {
		if dbauthz.IsNotAuthorizedError(err) {
			return nil, false, true, nil
		}
		return nil, false, false, xerrors.Errorf("get template file: %w", err)
	}
	files, truncated, err = ReadTerraformFiles(file.Data, TemplateSourceMaxBytes)
	if err != nil {
		return nil, false, false, err
	}
	return files, truncated, false, nil
}

// ReadTerraformFiles extracts *.tf and *.tfvars files from a tar archive,
// stopping once maxBytes of content has been collected. Hidden directories
// such as .terraform are skipped.
func ReadTerraformFiles(tarData []byte, maxBytes int) ([]SourceFile, bool, error) {
	var files []SourceFile
	remaining := maxBytes
	truncated := false
	reader := tar.NewReader(bytes.NewReader(tarData))
	for {
		header, err := reader.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, false, xerrors.Errorf("read template archive: %w", err)
		}
		if header.Typeflag != tar.TypeReg {
			continue
		}
		name := path.Clean(header.Name)
		if !IsTerraformPath(name) {
			continue
		}
		if remaining <= 0 {
			truncated = true
			break
		}
		// Read at most remaining+1 so we can tell whether the file was cut.
		limited := io.LimitReader(reader, int64(remaining)+1)
		content, err := io.ReadAll(limited)
		if err != nil {
			return nil, false, xerrors.Errorf("read %s: %w", name, err)
		}
		fileTruncated := false
		if len(content) > remaining {
			content = content[:remaining]
			fileTruncated = true
			truncated = true
		}
		remaining -= len(content)
		files = append(files, SourceFile{Path: name, Content: string(content), Truncated: fileTruncated})
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	return files, truncated, nil
}

// IsTerraformPath reports whether a template file path is Terraform source
// worth showing to the model.
func IsTerraformPath(name string) bool {
	for _, part := range strings.Split(name, "/") {
		if strings.HasPrefix(part, ".") && part != "." {
			return false
		}
	}
	return strings.HasSuffix(name, ".tf") || strings.HasSuffix(name, ".tfvars")
}

var errorMarkers = []string{"error", "failed", "fatal", "denied", "not found", "timeout", "timed out", "exit status"}

// splitBuildLogs returns earlier error-looking lines and the tail. Lines in
// the tail are not repeated in the error list.
func splitBuildLogs(logs []database.ProvisionerJobLog) (errorLines, tail []LogLine) {
	tailStart := len(logs) - BuildLogTailLines
	if tailStart < 0 {
		tailStart = 0
	}
	for i, l := range logs {
		line := LogLine{ID: l.ID, Stage: l.Stage, Level: string(l.Level), Output: l.Output}
		if i >= tailStart {
			tail = append(tail, line)
			continue
		}
		if len(errorLines) >= BuildLogErrorLines {
			continue
		}
		lower := strings.ToLower(l.Output)
		if l.Level == database.LogLevelError {
			errorLines = append(errorLines, line)
			continue
		}
		for _, marker := range errorMarkers {
			if strings.Contains(lower, marker) {
				errorLines = append(errorLines, line)
				break
			}
		}
	}
	return errorLines, tail
}

func writeLogLines(sb *strings.Builder, lines []LogLine) {
	for _, l := range lines {
		if l.Stage != "" {
			_, _ = fmt.Fprintf(sb, "[%s] ", l.Stage)
		}
		if l.Level != "" && l.Level != "info" {
			_, _ = fmt.Fprintf(sb, "%s: ", strings.ToUpper(l.Level))
		}
		_, _ = sb.WriteString(strings.TrimRight(l.Output, "\n"))
		_, _ = sb.WriteString("\n")
	}
}

var sensitiveParameterMarkers = []string{"secret", "token", "password", "passwd", "key", "credential"}

// redactParameter hides values whose names suggest credentials. Template
// parameters are not marked sensitive in the database, so the name is the only
// signal available.
func redactParameter(name, value string) string {
	lower := strings.ToLower(name)
	for _, marker := range sensitiveParameterMarkers {
		if strings.Contains(lower, marker) {
			return "[redacted]"
		}
	}
	return value
}

func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return strings.TrimSpace(s[:i])
	}
	return s
}

func indent(s, prefix string) string {
	lines := strings.Split(s, "\n")
	for i := range lines {
		lines[i] = prefix + lines[i]
	}
	return strings.Join(lines, "\n")
}
