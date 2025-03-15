package agentclaude

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/spf13/afero"
)

func New(ctx context.Context, apiKey, systemPrompt, taskPrompt string) error {
	claudePath, err := exec.LookPath("claude")
	if err != nil {
		return fmt.Errorf("claude not found: %w", err)
	}
	fs := afero.NewOsFs()
	err = injectClaudeMD(fs, `You are an AI agent in a Coder Workspace.

The user is running this task entirely autonomously.

You must use the coder-agent MCP server to periodically report your progress.
If you do not, the user will not be able to see your progress.
`, systemPrompt, "")
	if err != nil {
		return fmt.Errorf("failed to inject claude md: %w", err)
	}

	wd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get working directory: %w", err)
	}

	err = configureClaude(fs, ClaudeConfig{
		ConfigPath:       "",
		ProjectDirectory: wd,
		APIKey:           apiKey,
		AllowedTools:     []string{},
		MCPServers:       map[string]ClaudeConfigMCP{
			// "coder-agent": {
			// 	Command: "coder",
			// 	Args:    []string{"agent", "mcp"},
			// },
		},
	})
	if err != nil {
		return fmt.Errorf("failed to configure claude: %w", err)
	}

	cmd := exec.CommandContext(ctx, claudePath, taskPrompt)

	handlePause := func() {
		// We need to notify the user that we've paused!
		fmt.Println("We would normally notify the user...")
	}

	// Create a simple wrapper that starts monitoring only after first write
	stdoutWriter := &delayedPauseWriter{
		writer:      os.Stdout,
		pauseWindow: 2 * time.Second,
		onPause:     handlePause,
	}
	stderrWriter := &delayedPauseWriter{
		writer:      os.Stderr,
		pauseWindow: 2 * time.Second,
		onPause:     handlePause,
	}

	cmd.Stdout = stdoutWriter
	cmd.Stderr = stderrWriter
	cmd.Stdin = os.Stdin

	return cmd.Run()
}

// delayedPauseWriter wraps an io.Writer and only starts monitoring for pauses after first write
type delayedPauseWriter struct {
	writer        io.Writer
	pauseWindow   time.Duration
	onPause       func()
	lastWrite     time.Time
	mu            sync.Mutex
	started       bool
	pauseNotified bool
}

// Write implements io.Writer and starts monitoring on first write
func (w *delayedPauseWriter) Write(p []byte) (n int, err error) {
	w.mu.Lock()
	firstWrite := !w.started
	w.started = true
	w.lastWrite = time.Now()

	// Reset pause notification state when new output appears
	w.pauseNotified = false

	w.mu.Unlock()

	// Start monitoring goroutine on first write
	if firstWrite {
		go w.monitorPauses()
	}

	return w.writer.Write(p)
}

// monitorPauses checks for pauses in writing and calls onPause when detected
func (w *delayedPauseWriter) monitorPauses() {
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for range ticker.C {
		w.mu.Lock()
		elapsed := time.Since(w.lastWrite)
		alreadyNotified := w.pauseNotified

		// If we detect a pause and haven't notified yet, mark as notified
		if elapsed >= w.pauseWindow && !alreadyNotified {
			w.pauseNotified = true
		}

		w.mu.Unlock()

		// Only notify once per pause period
		if elapsed >= w.pauseWindow && !alreadyNotified {
			w.onPause()
		}
	}
}

func injectClaudeMD(fs afero.Fs, coderPrompt, systemPrompt string, configPath string) error {
	if configPath == "" {
		configPath = filepath.Join(os.Getenv("HOME"), ".claude", "CLAUDE.md")
	}
	_, err := fs.Stat(configPath)
	if err != nil {
		if !os.IsNotExist(err) {
			return fmt.Errorf("failed to stat claude config: %w", err)
		}
	}
	content := ""
	if err == nil {
		contentBytes, err := afero.ReadFile(fs, configPath)
		if err != nil {
			return fmt.Errorf("failed to read claude config: %w", err)
		}
		content = string(contentBytes)
	}

	// Define the guard strings
	const coderPromptStartGuard = "<coder-prompt>"
	const coderPromptEndGuard = "</coder-prompt>"
	const systemPromptStartGuard = "<system-prompt>"
	const systemPromptEndGuard = "</system-prompt>"

	// Extract the content without the guarded sections
	cleanContent := content

	// Remove existing coder prompt section if it exists
	coderStartIdx := indexOf(cleanContent, coderPromptStartGuard)
	coderEndIdx := indexOf(cleanContent, coderPromptEndGuard)
	if coderStartIdx != -1 && coderEndIdx != -1 && coderStartIdx < coderEndIdx {
		beforeCoderPrompt := cleanContent[:coderStartIdx]
		afterCoderPrompt := cleanContent[coderEndIdx+len(coderPromptEndGuard):]
		cleanContent = beforeCoderPrompt + afterCoderPrompt
	}

	// Remove existing system prompt section if it exists
	systemStartIdx := indexOf(cleanContent, systemPromptStartGuard)
	systemEndIdx := indexOf(cleanContent, systemPromptEndGuard)
	if systemStartIdx != -1 && systemEndIdx != -1 && systemStartIdx < systemEndIdx {
		beforeSystemPrompt := cleanContent[:systemStartIdx]
		afterSystemPrompt := cleanContent[systemEndIdx+len(systemPromptEndGuard):]
		cleanContent = beforeSystemPrompt + afterSystemPrompt
	}

	// Trim any leading whitespace from the clean content
	cleanContent = strings.TrimSpace(cleanContent)

	// Create the new content with both prompts prepended
	var newContent string

	// Add coder prompt
	newContent = coderPromptStartGuard + "\n" + coderPrompt + "\n" + coderPromptEndGuard + "\n\n"

	// Add system prompt
	newContent += systemPromptStartGuard + "\n" + systemPrompt + "\n" + systemPromptEndGuard + "\n\n"

	// Add the rest of the content
	if cleanContent != "" {
		newContent += cleanContent
	}

	// Write the updated content back to the file
	err = afero.WriteFile(fs, configPath, []byte(newContent), 0644)
	if err != nil {
		return fmt.Errorf("failed to write claude config: %w", err)
	}

	return nil
}

// indexOf returns the index of the first instance of substr in s,
// or -1 if substr is not present in s.
func indexOf(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}

type ClaudeConfig struct {
	ConfigPath       string
	ProjectDirectory string
	APIKey           string
	AllowedTools     []string
	MCPServers       map[string]ClaudeConfigMCP
}

type ClaudeConfigMCP struct {
	Command string
	Args    []string
	Env     map[string]string
}

func configureClaude(fs afero.Fs, cfg ClaudeConfig) error {
	if cfg.ConfigPath == "" {
		cfg.ConfigPath = filepath.Join(os.Getenv("HOME"), ".claude.json")
	}
	var config map[string]any
	_, err := fs.Stat(cfg.ConfigPath)
	if err != nil {
		if os.IsNotExist(err) {
			config = make(map[string]any)
			err = nil
		} else {
			return fmt.Errorf("failed to stat claude config: %w", err)
		}
	}
	if err == nil {
		jsonBytes, err := afero.ReadFile(fs, cfg.ConfigPath)
		if err != nil {
			return fmt.Errorf("failed to read claude config: %w", err)
		}
		err = json.Unmarshal(jsonBytes, &config)
		if err != nil {
			return fmt.Errorf("failed to unmarshal claude config: %w", err)
		}
	}

	if cfg.APIKey != "" {
		// Stops Claude from requiring the user to generate
		// a Claude-specific API key.
		config["primaryApiKey"] = cfg.APIKey
	}
	// Stops Claude from asking for onboarding.
	config["hasCompletedOnboarding"] = true
	// Stops Claude from asking for permissions.
	config["bypassPermissionsModeAccepted"] = true

	projects, ok := config["projects"].(map[string]any)
	if !ok {
		projects = make(map[string]any)
	}

	project, ok := projects[cfg.ProjectDirectory].(map[string]any)
	if !ok {
		project = make(map[string]any)
	}

	allowedTools, ok := project["allowedTools"].([]string)
	if !ok {
		allowedTools = []string{}
	}

	// Add cfg.AllowedTools to the list if they're not already present.
	for _, tool := range cfg.AllowedTools {
		for _, existingTool := range allowedTools {
			if tool == existingTool {
				continue
			}
		}
		allowedTools = append(allowedTools, tool)
	}
	project["allowedTools"] = allowedTools

	mcpServers, ok := project["mcpServers"].(map[string]any)
	if !ok {
		mcpServers = make(map[string]any)
	}
	for name, mcp := range cfg.MCPServers {
		mcpServers[name] = mcp
	}
	project["mcpServers"] = mcpServers
	// Prevents Claude from asking the user to complete the project onboarding.
	project["hasCompletedProjectOnboarding"] = true
	projects[cfg.ProjectDirectory] = project
	config["projects"] = projects

	jsonBytes, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal claude config: %w", err)
	}
	err = afero.WriteFile(fs, cfg.ConfigPath, jsonBytes, 0644)
	if err != nil {
		return fmt.Errorf("failed to write claude config: %w", err)
	}
	return nil
}
