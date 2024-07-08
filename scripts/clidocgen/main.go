package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/coder/coder/v2/enterprise/cli"
	"github.com/coder/flog"
	"github.com/coder/serpent"
)

// route is an individual page object in the docs manifest.json.
type route struct {
	Title       string  `json:"title,omitempty"`
	Description string  `json:"description,omitempty"`
	Path        string  `json:"path,omitempty"`
	IconPath    string  `json:"icon_path,omitempty"`
	State       string  `json:"state,omitempty"`
	Children    []route `json:"children,omitempty"`
}

// manifest describes the entire documentation index.
type manifest struct {
	Versions []string `json:"versions,omitempty"`
	Routes   []route  `json:"routes,omitempty"`
}

func prepareEnv() {
	// Unset CODER_ environment variables
	for _, env := range os.Environ() {
		if strings.HasPrefix(env, "CODER_") {
			split := strings.SplitN(env, "=", 2)
			if err := os.Unsetenv(split[0]); err != nil {
				panic(err)
			}
		}
	}

	// Override default OS values to ensure the same generated results.
	err := os.Setenv("CLIDOCGEN_CACHE_DIRECTORY", "~/.cache")
	if err != nil {
		panic(err)
	}
	err = os.Setenv("CLIDOCGEN_CONFIG_DIRECTORY", "~/.config/coderv2")
	if err != nil {
		panic(err)
	}
}

func deleteEmptyDirs(dir string) error {
	return filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			return nil
		}
		ents, err := os.ReadDir(path)
		if err != nil {
			return err
		}
		if len(ents) == 0 {
			flog.Infof("deleting empty dir\t %v", path)
			err = os.Remove(path)
			if err != nil {
				return err
			}
		}
		return nil
	})
}

func main() {
	prepareEnv()

	workdir, err := os.Getwd()
	if err != nil {
		flog.Fatalf("getwd: %v", err)
	}
	root := (&cli.RootCmd{})

	// wroteMap indexes file paths to commands.
	wroteMap := make(map[string]*serpent.Command)

	var (
		docsDir        = filepath.Join(workdir, "docs")
		cliMarkdownDir = filepath.Join(docsDir, "cli")
	)

	cmd, err := root.Command(root.EnterpriseSubcommands())
	if err != nil {
		flog.Fatalf("creating command: %v", err)
	}
	err = genTree(
		cliMarkdownDir,
		cmd,
		wroteMap,
	)
	if err != nil {
		flog.Fatalf("generating markdowns: %v", err)
	}

	// Delete old files
	err = filepath.Walk(cliMarkdownDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		_, ok := wroteMap[path]
		if !ok {
			flog.Infof("deleting old doc\t %v", path)
			if err := os.Remove(path); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		flog.Fatalf("deleting old docs: %v", err)
	}

	err = deleteEmptyDirs(cliMarkdownDir)
	if err != nil {
		flog.Fatalf("deleting empty dirs: %v", err)
	}

	// Update manifest
	manifestPath := filepath.Join(docsDir, "manifest.json")

	manifestByt, err := os.ReadFile(manifestPath)
	if err != nil {
		flog.Fatalf("reading manifest: %v", err)
	}

	var manifest manifest
	err = json.Unmarshal(manifestByt, &manifest)
	if err != nil {
		flog.Fatalf("unmarshalling manifest: %v", err)
	}

	var found bool
	for i := range manifest.Routes {
		rt := &manifest.Routes[i]
		if rt.Title != "Command Line" {
			continue
		}
		rt.Children = nil
		found = true
		for path, cmd := range wroteMap {
			relPath, err := filepath.Rel(docsDir, path)
			if err != nil {
				flog.Fatalf("getting relative path: %v", err)
			}
			rt.Children = append(rt.Children, route{
				Title:       fullName(cmd),
				Description: cmd.Short,
				Path:        relPath,
			})
		}
		// Sort children by title because wroteMap iteration is
		// non-deterministic.
		sort.Slice(rt.Children, func(i, j int) bool {
			return rt.Children[i].Title < rt.Children[j].Title
		})
	}

	if !found {
		flog.Fatalf("could not find Command Line route in manifest")
	}

	manifestByt, err = json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		flog.Fatalf("marshaling manifest: %v", err)
	}

	err = os.WriteFile(manifestPath, manifestByt, 0o600)
	if err != nil {
		flog.Fatalf("writing manifest: %v", err)
	}
}
