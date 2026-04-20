package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strings"
	"time"

	"golang.org/x/xerrors"
	"gopkg.in/yaml.v3"
)

// Config represents the top-level YAML configuration.
type Config struct {
	Clients []ClientConfig `yaml:"clients"`
	Prompts []PromptConfig `yaml:"prompts"`
}

// ClientConfig defines how to invoke a CLI client.
type ClientConfig struct {
	Name         string            `yaml:"name"`
	Command      []string          `yaml:"command"`
	Env          map[string]string `yaml:"env"`
	OutputFormat string            `yaml:"output_format"`
	ErrorField   string            `yaml:"error_field"`
}

// PromptConfig defines a test prompt.
type PromptConfig struct {
	Name     string `yaml:"name"`
	Text     string `yaml:"text"`
	Category string `yaml:"category"`
}

// TestResult captures the outcome of a single client × prompt test.
type TestResult struct {
	Client   string `json:"client"`
	Prompt   string `json:"prompt"`
	Passed   bool   `json:"passed"`
	Error    string `json:"error,omitempty"`
	Duration string `json:"duration"`
}

const defaultTimeout = 120 * time.Second

func main() {
	var (
		configPath string
		clientFlag string
		promptFlag string
		timeout    time.Duration
		jsonOutput bool
	)

	flag.StringVar(&configPath, "config", "config.yaml", "Path to YAML config file")
	flag.StringVar(&clientFlag, "client", "", "Run only this client (empty = all)")
	flag.StringVar(&promptFlag, "prompt", "", "Run only this prompt (empty = all)")
	flag.DurationVar(&timeout, "timeout", defaultTimeout, "Timeout per test invocation")
	flag.BoolVar(&jsonOutput, "json", true, "Output results as JSON")
	flag.Parse()

	cfg, err := loadConfig(configPath)
	if err != nil {
		log.Fatal("failed to load config: ", err)
	}

	if err := validateEnv(cfg, clientFlag); err != nil {
		log.Fatal("environment validation failed: ", err)
	}

	results := runTests(cfg, clientFlag, promptFlag, timeout)

	if jsonOutput {
		if err := printResultsJSON(results); err != nil {
			log.Fatal("failed to encode results: ", err)
		}
	} else {
		printResultsTable(results)
	}

	for _, r := range results {
		if !r.Passed {
			os.Exit(1)
		}
	}
}

// loadConfig reads and parses the YAML config file.
func loadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, xerrors.Errorf("reading %s: %w", path, err)
	}
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, xerrors.Errorf("parsing %s: %w", path, err)
	}
	if len(cfg.Clients) == 0 {
		return nil, xerrors.Errorf("no clients defined in %s", path)
	}
	if len(cfg.Prompts) == 0 {
		return nil, xerrors.Errorf("no prompts defined in %s", path)
	}
	return &cfg, nil
}

// validateEnv checks that required environment variables are set for
// the clients we're about to run.
func validateEnv(cfg *Config, clientFilter string) error {
	var missing []string
	for _, client := range cfg.Clients {
		if clientFilter != "" && client.Name != clientFilter {
			continue
		}
		for _, envVar := range client.Env {
			if os.Getenv(envVar) == "" {
				missing = append(missing, envVar)
			}
		}
	}
	if len(missing) > 0 {
		return xerrors.Errorf("missing environment variables: %s", strings.Join(missing, ", "))
	}
	return nil
}

// runTests executes each client × prompt combination, logs errors
// as they happen, and returns collected results for the summary.
func runTests(cfg *Config, clientFilter, promptFilter string, timeout time.Duration) []TestResult {
	var results []TestResult
	loggedVersions := make(map[string]bool)
	for _, client := range cfg.Clients {
		if clientFilter != "" && client.Name != clientFilter {
			continue
		}
		if !loggedVersions[client.Name] {
			logClientVersion(client)
			loggedVersions[client.Name] = true
		}
		for _, prompt := range cfg.Prompts {
			if promptFilter != "" && prompt.Name != promptFilter {
				continue
			}
			log.Printf("running: client=%s prompt=%s", client.Name, prompt.Name)
			result := runSingleTest(client, prompt, timeout)
			if !result.Passed {
				log.Printf("FAIL: client=%s prompt=%s\n%s", client.Name, prompt.Name, result.Error)
			} else {
				log.Printf("PASS: client=%s prompt=%s (%s)", client.Name, prompt.Name, result.Duration)
			}
			results = append(results, result)
		}
	}
	return results
}

// logClientVersion runs "<command> --version" and logs the output.
func logClientVersion(client ClientConfig) {
	//nolint:gosec // Command comes from trusted YAML config.
	out, err := exec.Command(client.Command[0], "--version").Output()
	if err != nil {
		log.Printf("client=%s version=unknown", client.Name)
		return
	}
	log.Printf("client=%s version=%s", client.Name, strings.TrimSpace(string(out)))
}

// runSingleTest invokes one CLI client with one prompt and returns
// the full error output on failure.
func runSingleTest(
	client ClientConfig,
	prompt PromptConfig,
	timeout time.Duration,
) TestResult {
	start := time.Now()
	result := TestResult{
		Client: client.Name,
		Prompt: prompt.Name,
	}

	// Build command: copy configured args, then append the prompt.
	args := make([]string, 0, len(client.Command))
	args = append(args, client.Command[1:]...)
	args = append(args, prompt.Text)

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	//nolint:gosec // Command arguments come from trusted YAML config.
	cmd := exec.CommandContext(ctx, client.Command[0], args...)

	// Pass through current environment plus client-specific vars.
	cmd.Env = os.Environ()
	for _, envVar := range client.Env {
		val := os.Getenv(envVar)
		if val != "" {
			cmd.Env = append(cmd.Env, envVar+"="+val)
		}
	}

	stdout, err := cmd.Output()
	result.Duration = time.Since(start).Round(time.Millisecond).String()

	if ctx.Err() != nil {
		result.Error = fmt.Sprintf("timeout after %s", timeout)
		return result
	}

	if err != nil {
		result.Error = collectError(err, stdout)
		return result
	}

	// Check output for error indicators (e.g. Claude's "is_error"
	// field in JSON output).
	if client.OutputFormat == "json" && client.ErrorField != "" {
		if hasErrorInJSON(stdout, client.ErrorField) {
			result.Error = fmt.Sprintf(
				"error field %q is true in output: %s",
				client.ErrorField,
				strings.TrimSpace(string(stdout)),
			)
			return result
		}
	}

	result.Passed = true
	return result
}

// collectError builds a complete error message from a command
// failure, including exit code, stderr, and stdout.
func collectError(err error, stdout []byte) string {
	var parts []string

	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		parts = append(parts, fmt.Sprintf("exit code %d", exitErr.ExitCode()))
		if len(exitErr.Stderr) > 0 {
			parts = append(parts, "stderr: "+strings.TrimSpace(string(exitErr.Stderr)))
		}
	} else {
		parts = append(parts, err.Error())
	}

	if len(stdout) > 0 {
		parts = append(parts, "stdout: "+strings.TrimSpace(string(stdout)))
	}

	return strings.Join(parts, "\n")
}

// hasErrorInJSON checks whether a named boolean field in the JSON
// output is true. Used for Claude's "is_error" field.
func hasErrorInJSON(data []byte, field string) bool {
	var parsed map[string]interface{}
	if err := json.Unmarshal(data, &parsed); err != nil {
		return false
	}
	val, ok := parsed[field]
	if !ok {
		return false
	}
	boolVal, ok := val.(bool)
	return ok && boolVal
}

// printResultsJSON writes the results as formatted JSON to stdout.
func printResultsJSON(results []TestResult) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(results)
}

// printResultsTable writes a clean summary table to stdout.
func printResultsTable(results []TestResult) {
	separator := strings.Repeat("-", 50)
	_, _ = fmt.Println()
	_, _ = fmt.Println("Summary:")
	_, _ = fmt.Println(separator)
	_, _ = fmt.Printf("%-12s %-14s %-8s %s\n",
		"CLIENT", "PROMPT", "STATUS", "DURATION")
	_, _ = fmt.Println(separator)

	passed, failed := 0, 0
	for _, r := range results {
		status := "PASS"
		if !r.Passed {
			status = "FAIL"
			failed++
		} else {
			passed++
		}
		_, _ = fmt.Printf("%-12s %-14s %-8s %s\n",
			r.Client, r.Prompt, status, r.Duration)
	}

	_, _ = fmt.Println(separator)
	_, _ = fmt.Printf("Total: %d passed, %d failed\n\n", passed, failed)
}
