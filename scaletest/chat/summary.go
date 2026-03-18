package chat

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"runtime/debug"
	"time"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/buildinfo"
	"github.com/coder/coder/v2/scaletest/harness"
)

type WorkspaceMode string

const (
	WorkspaceModeSharedWorkspace WorkspaceMode = "shared_workspace"
	WorkspaceModeTemplate        WorkspaceMode = "template"
)

type SummaryConfig struct {
	RunID                 string
	WorkspaceMode         WorkspaceMode
	WorkspaceID           *uuid.UUID
	TemplateID            *uuid.UUID
	TemplateName          string
	CreatedWorkspaceCount int64
	ModelConfigID         *uuid.UUID
	Count                 int64
	Turns                 int
	Prompt                string
	FollowUpPrompt        string
	FollowUpStartDelay    time.Duration
	LLMMockURL            string
	OutputSpecs           []string
}

type Summary struct {
	RunID                     string         `json:"run_id"`
	StartedAt                 time.Time      `json:"started_at"`
	CompletedAt               time.Time      `json:"completed_at"`
	FollowUpPhaseReleasedAt   *time.Time     `json:"follow_up_phase_released_at,omitempty"`
	WorkspaceMode             WorkspaceMode  `json:"workspace_mode"`
	WorkspaceID               *uuid.UUID     `json:"workspace_id,omitempty"`
	TemplateID                *uuid.UUID     `json:"template_id,omitempty"`
	TemplateName              string         `json:"template_name,omitempty"`
	CreatedWorkspaceCount     int64          `json:"created_workspace_count,omitempty"`
	ModelConfigID             *uuid.UUID     `json:"model_config_id,omitempty"`
	Count                     int64          `json:"count"`
	Turns                     int            `json:"turns"`
	PromptFingerprint         string         `json:"prompt_fingerprint"`
	PromptLength              int            `json:"prompt_length"`
	FollowUpPromptFingerprint string         `json:"follow_up_prompt_fingerprint,omitempty"`
	FollowUpPromptLength      int            `json:"follow_up_prompt_length,omitempty"`
	LLMMockURL                string         `json:"llm_mock_url,omitempty"`
	FollowUpDelayEnabled      bool           `json:"follow_up_delay_enabled"`
	FollowUpStartDelay        string         `json:"follow_up_start_delay"`
	FollowUpStartDelayMS      int64          `json:"follow_up_start_delay_ms"`
	RawOutputSpecs            []string       `json:"raw_output_specs,omitempty"`
	Results                   ResultsSummary `json:"results"`
	Build                     BuildSummary   `json:"build"`
}

type ResultsSummary struct {
	TotalRuns int    `json:"total_runs"`
	TotalPass int    `json:"total_pass"`
	TotalFail int    `json:"total_fail"`
	Elapsed   string `json:"elapsed"`
	ElapsedMS int64  `json:"elapsed_ms"`
}

type BuildSummary struct {
	Version     string     `json:"version"`
	ExternalURL string     `json:"external_url,omitempty"`
	Revision    string     `json:"revision,omitempty"`
	Time        *time.Time `json:"time,omitempty"`
}

func NewSummary(cfg SummaryConfig, results harness.Results, startedAt, completedAt time.Time, followUpPhaseReleasedAt *time.Time) Summary {
	summary := Summary{
		RunID:                 cfg.RunID,
		StartedAt:             startedAt.UTC(),
		CompletedAt:           completedAt.UTC(),
		WorkspaceMode:         cfg.WorkspaceMode,
		TemplateName:          cfg.TemplateName,
		CreatedWorkspaceCount: cfg.CreatedWorkspaceCount,
		ModelConfigID:         cfg.ModelConfigID,
		Count:                 cfg.Count,
		Turns:                 cfg.Turns,
		PromptFingerprint:     promptFingerprint(cfg.Prompt),
		PromptLength:          len(cfg.Prompt),
		LLMMockURL:            cfg.LLMMockURL,
		FollowUpDelayEnabled:  cfg.FollowUpStartDelay > 0,
		FollowUpStartDelay:    cfg.FollowUpStartDelay.String(),
		FollowUpStartDelayMS:  cfg.FollowUpStartDelay.Milliseconds(),
		RawOutputSpecs:        append([]string(nil), cfg.OutputSpecs...),
		Results: ResultsSummary{
			TotalRuns: results.TotalRuns,
			TotalPass: results.TotalPass,
			TotalFail: results.TotalFail,
			Elapsed:   time.Duration(results.Elapsed).String(),
			ElapsedMS: results.ElapsedMS,
		},
		Build: buildSummary(),
	}

	if cfg.WorkspaceID != nil {
		workspaceID := *cfg.WorkspaceID
		summary.WorkspaceID = &workspaceID
	}
	if cfg.TemplateID != nil {
		templateID := *cfg.TemplateID
		summary.TemplateID = &templateID
	}

	if cfg.Turns > 1 {
		summary.FollowUpPromptFingerprint = promptFingerprint(cfg.FollowUpPrompt)
		summary.FollowUpPromptLength = len(cfg.FollowUpPrompt)
	}

	if followUpPhaseReleasedAt != nil {
		releasedAt := followUpPhaseReleasedAt.UTC()
		summary.FollowUpPhaseReleasedAt = &releasedAt
	}

	return summary
}

func (s Summary) CompactJSON() ([]byte, error) {
	data, err := json.Marshal(s)
	if err != nil {
		return nil, xerrors.Errorf("marshal compact summary: %w", err)
	}
	return data, nil
}

func (s Summary) Write(path string) error {
	f, err := os.Create(path)
	if err != nil {
		return xerrors.Errorf("create summary file: %w", err)
	}
	defer f.Close()

	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	if err := enc.Encode(s); err != nil {
		return xerrors.Errorf("encode summary JSON: %w", err)
	}
	if err := f.Sync(); err != nil {
		return xerrors.Errorf("sync summary file: %w", err)
	}
	return nil
}

func buildSummary() BuildSummary {
	summary := BuildSummary{
		Version:     buildinfo.Version(),
		ExternalURL: buildinfo.ExternalURL(),
		Revision:    readBuildSetting("vcs.revision"),
	}
	if buildTime, ok := buildinfo.Time(); ok {
		buildTimeUTC := buildTime.UTC()
		summary.Time = &buildTimeUTC
	}
	return summary
}

func promptFingerprint(prompt string) string {
	if prompt == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(prompt))
	return hex.EncodeToString(sum[:])
}

func readBuildSetting(key string) string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return ""
	}
	for _, setting := range info.Settings {
		if setting.Key == key {
			return setting.Value
		}
	}
	return ""
}
