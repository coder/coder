package main

import (
	"context"
	"flag"
	"log"
	"os"
	"path/filepath"
	"sort"
)

// ModuleConfig defines the builder catalog metadata that cannot be
// inferred from the registry (category, OS compatibility, conflicts).
type ModuleConfig struct {
	Category      string   `json:"category"`
	CompatibleOS  []string `json:"compatible_os"`
	ConflictsWith []string `json:"conflicts_with"`
	SkipVars      []string `json:"skip_vars,omitempty"`
}

// moduleConfigs defines the builder-specific metadata for each module.
var moduleConfigs = map[string]ModuleConfig{
	"code-server":        {Category: "IDE", CompatibleOS: []string{"linux"}, ConflictsWith: []string{"vscode-web"}},
	"jetbrains":          {Category: "IDE", CompatibleOS: []string{"linux"}, ConflictsWith: []string{}},
	"vscode-desktop":     {Category: "IDE", CompatibleOS: []string{"linux"}, ConflictsWith: []string{}},
	"vscode-web":         {Category: "IDE", CompatibleOS: []string{"linux"}, ConflictsWith: []string{"code-server"}},
	"cursor":             {Category: "IDE", CompatibleOS: []string{"linux"}, ConflictsWith: []string{}},
	"windsurf":           {Category: "IDE", CompatibleOS: []string{"linux"}, ConflictsWith: []string{}},
	"zed":                {Category: "IDE", CompatibleOS: []string{"linux"}, ConflictsWith: []string{}},
	"kiro":               {Category: "IDE", CompatibleOS: []string{"linux"}, ConflictsWith: []string{}},
	"claude-code":        {Category: "AI Agent", CompatibleOS: []string{"linux"}, ConflictsWith: []string{}},
	"aider":              {Category: "AI Agent", CompatibleOS: []string{"linux"}, ConflictsWith: []string{}},
	"goose":              {Category: "AI Agent", CompatibleOS: []string{"linux"}, ConflictsWith: []string{}},
	"amazon-q":           {Category: "AI Agent", CompatibleOS: []string{"linux"}, ConflictsWith: []string{}},
	"git-clone":          {Category: "Source Control", CompatibleOS: []string{"linux"}, ConflictsWith: []string{}},
	"git-config":         {Category: "Source Control", CompatibleOS: []string{"linux"}, ConflictsWith: []string{}},
	"git-commit-signing": {Category: "Source Control", CompatibleOS: []string{"linux"}, ConflictsWith: []string{}},
	"dotfiles":           {Category: "Utility", CompatibleOS: []string{"linux"}, ConflictsWith: []string{}},
	"personalize":        {Category: "Utility", CompatibleOS: []string{"linux"}, ConflictsWith: []string{}},
	"filebrowser":        {Category: "Utility", CompatibleOS: []string{"linux"}, ConflictsWith: []string{}},
	"jupyterlab":         {Category: "Utility", CompatibleOS: []string{"linux"}, ConflictsWith: []string{}},
}

func main() {
	outputPath := flag.String("output", "", "Output directory for generated module files (required)")
	baseURL := flag.String("registry-url", registryBaseURL, "Base URL of the Coder registry API")
	flag.Parse()

	if *outputPath == "" {
		flag.Usage()
		os.Exit(1)
	}

	ctx := context.Background()
	moduleIDs := sortedKeys(moduleConfigs)
	var failures int

	for _, id := range moduleIDs {
		cfg := moduleConfigs[id]
		registryID := "coder/" + id
		log.Printf("Generating %s...", id)

		regMod, err := fetchModule(ctx, *baseURL, registryID)
		if err != nil {
			log.Printf("  ERROR fetching module: %v", err)
			failures++
			continue
		}

		version, err := fetchLatestVersion(ctx, *baseURL, "coder", id)
		if err != nil {
			log.Printf("  WARNING: could not determine version: %v", err)
			version = "0.0.0"
		}

		vars := convertVariables(regMod.Variables, cfg.SkipVars)

		manifest := ModuleManifest{
			ID:            id,
			DisplayName:   regMod.DisplayName,
			Description:   regMod.Description,
			Icon:          normalizeIcon(regMod.IconURL),
			Category:      cfg.Category,
			Tags:          regMod.Tags,
			CompatibleOS:  cfg.CompatibleOS,
			ConflictsWith: cfg.ConflictsWith,
			PinnedVersion: version,
			Variables:     vars,
		}

		outDir := filepath.Join(*outputPath, id)
		if err := os.MkdirAll(outDir, 0o755); err != nil {
			log.Printf("  ERROR creating directory: %v", err)
			failures++
			continue
		}

		if err := writeModuleJSON(filepath.Join(outDir, "module.json"), manifest); err != nil {
			log.Printf("  ERROR writing module.json: %v", err)
			failures++
			continue
		}

		if err := writeTFTmpl(filepath.Join(outDir, id+".tf.tmpl"), manifest); err != nil {
			log.Printf("  ERROR writing .tf.tmpl: %v", err)
			failures++
			continue
		}

		log.Printf("  OK: %d variables, version %s", len(vars), version)
	}

	if failures > 0 {
		log.Fatalf("Failed to generate %d module(s)", failures)
	}
}

func sortedKeys(m map[string]ModuleConfig) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
