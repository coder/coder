package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/xerrors"
)

// ModuleConfig defines the builder catalog metadata that cannot be
// inferred from the registry (category, OS compatibility, conflicts).
type ModuleConfig struct {
	Category      string   `json:"category"`
	CompatibleOS  []string `json:"compatible_os"`
	ConflictsWith []string `json:"conflicts_with"`
	SkipVars      []string `json:"skip_vars,omitempty"`
	// Namespace is the registry namespace (e.g. "coder" or "coder-labs").
	// When empty, defaults to "coder".
	Namespace string `json:"namespace,omitempty"`
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
	"codex":              {Category: "AI Agent", CompatibleOS: []string{"linux"}, ConflictsWith: []string{}, Namespace: "coder-labs"},
	"aider":              {Category: "AI Agent", CompatibleOS: []string{"linux"}, ConflictsWith: []string{}},
	"amazon-q":           {Category: "AI Agent", CompatibleOS: []string{"linux"}, ConflictsWith: []string{}},
	"antigravity":        {Category: "AI Agent", CompatibleOS: []string{"linux"}, ConflictsWith: []string{}},
	"git-clone":          {Category: "Source Control", CompatibleOS: []string{"linux"}, ConflictsWith: []string{}},
	"git-config":         {Category: "Source Control", CompatibleOS: []string{"linux"}, ConflictsWith: []string{}},
	"git-commit-signing": {Category: "Source Control", CompatibleOS: []string{"linux"}, ConflictsWith: []string{}},
	"dotfiles":           {Category: "Utility", CompatibleOS: []string{"linux"}, ConflictsWith: []string{}},
	"personalize":        {Category: "Utility", CompatibleOS: []string{"linux"}, ConflictsWith: []string{}},
	"filebrowser":        {Category: "Utility", CompatibleOS: []string{"linux"}, ConflictsWith: []string{}},
	"jupyterlab":         {Category: "Utility", CompatibleOS: []string{"linux"}, ConflictsWith: []string{}},
	"kasmvnc":            {Category: "Utility", CompatibleOS: []string{"linux"}, ConflictsWith: []string{}},
}

func main() {
	outputPath := flag.String("output", "", "Output directory for generated module files (required)")
	baseURL := flag.String("registry-url", registryBaseURL, "Base URL of the Coder registry API")
	flag.Parse()

	if *outputPath == "" {
		flag.Usage()
		os.Exit(1)
	}

	args := flag.Args()
	if len(args) == 0 {
		log.Fatal("specify one or more registry module IDs to generate (e.g. coder/antigravity coder-labs/codex)")
	}
	refs, err := resolveModuleArgs(args)
	if err != nil {
		log.Fatal(err)
	}

	ctx := context.Background()
	var failures int

	for _, ref := range refs {
		id := ref.slug
		cfg := moduleConfigs[id]
		namespace := ref.namespace
		registryID := fmt.Sprintf("%s/%s", namespace, id)
		log.Printf("Generating %s...", registryID)

		regMod, err := fetchModule(ctx, *baseURL, registryID)
		if err != nil {
			log.Printf("  ERROR fetching module: %v", err)
			failures++
			continue
		}

		version, err := fetchLatestVersion(ctx, *baseURL, namespace, id)
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
			Namespace:     namespace,
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

// moduleRef is a resolved module reference with its registry namespace and slug.
type moduleRef struct {
	namespace string
	slug      string
}

// resolveModuleArgs parses CLI arguments in "namespace/slug" format and
// validates each against moduleConfigs. Returns an error if any argument
// is malformed, references an unknown module, or has a namespace that
// does not match the configured value.
func resolveModuleArgs(args []string) ([]moduleRef, error) {
	refs := make([]moduleRef, 0, len(args))
	for _, arg := range args {
		parts := strings.SplitN(arg, "/", 2)
		if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
			return nil, xerrors.Errorf("invalid module ID %q; expected namespace/slug (e.g. coder/antigravity)", arg)
		}
		namespace, slug := parts[0], parts[1]
		cfg, ok := moduleConfigs[slug]
		if !ok {
			return nil, xerrors.Errorf("unknown module %q; add it to moduleConfigs first", slug)
		}
		expectedNS := cfg.Namespace
		if expectedNS == "" {
			expectedNS = "coder"
		}
		if namespace != expectedNS {
			return nil, xerrors.Errorf("module %q namespace mismatch: got %q, moduleConfigs expects %q", slug, namespace, expectedNS)
		}
		refs = append(refs, moduleRef{namespace: namespace, slug: slug})
	}
	return refs, nil
}
