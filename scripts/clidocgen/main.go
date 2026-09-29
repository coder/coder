package main

import (
	"encoding/json"
	"flag"
	"os"
	"path/filepath"

	"github.com/coder/coder/v2/enterprise/cli"
	"github.com/coder/coder/v2/scripts/atomicwrite"
	"github.com/coder/coder/v2/scripts/docgenenv"
	"github.com/coder/flog"
	"github.com/coder/serpent"
)

// cliIndexRoute holds the "Command Line" manifest route's metadata so the
// generated index page can mirror it through the shared docgenenv.FrontMatter
// emitter (see the frontMatter template func in gen.go). main populates it
// before genTree runs.
var cliIndexRoute docgenenv.Route

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
	manifestOnly := flag.Bool("manifest-only", false, "Only rebuild the \"Command Line\" section of manifest.json; do not write reference pages.")
	flag.Parse()

	docgenenv.Prepare()

	workdir, err := os.Getwd()
	if err != nil {
		flog.Fatalf("getwd: %v", err)
	}
	root := (&cli.RootCmd{})

	var (
		docsDir        = filepath.Join(workdir, "docs")
		cliMarkdownDir = filepath.Join(docsDir, "reference/cli")
	)

	if d := os.Getenv("DOCS_DIR"); d != "" {
		docsDir = d
		cliMarkdownDir = filepath.Join(docsDir, "reference/cli")
	}

	// Load the manifest up front so the generated index page can mirror the
	// "Command Line" route's curated metadata (title/description/icon_path)
	// instead of the root command name.
	manifestPath := filepath.Join(docsDir, "manifest.json")
	man, err := docgenenv.LoadManifest(manifestPath)
	if err != nil {
		flog.Fatalf("%v", err)
	}
	cmdLine := man.FindRoute("Reference", "Command Line")
	if cmdLine == nil {
		flog.Fatalf("could not find Command Line route in manifest %q", manifestPath)
	}
	// Mirror the whole "Command Line" route (minus its nav children) so the
	// index page front matter carries every current and future per-page field
	// automatically, the same way the API index mirrors its manifest route.
	cliIndexRoute = cliIndexRouteFrom(*cmdLine)

	cmd, err := root.Command(root.EnterpriseSubcommands())
	if err != nil {
		flog.Fatalf("creating command: %v", err)
	}
	if !*manifestOnly {
		writePages(cliMarkdownDir, cmd)
	}

	// Rebuild the "Command Line" route's children from the command tree so
	// the nav nests the same way the generated pages do. cmdLine aliases the
	// manifest loaded above, so mutating it updates the manifest in place.
	cmdLine.Children = cliManifestChildren(cmd)

	manifestByt, err := json.MarshalIndent(man, "", "  ")
	if err != nil {
		flog.Fatalf("marshaling manifest: %v", err)
	}

	err = atomicwrite.File(manifestPath, manifestByt)
	if err != nil {
		flog.Fatalf("writing manifest: %v", err)
	}
}

// writePages generates a reference page for every visible command under
// cliMarkdownDir, then deletes pages and directories left over from commands
// that no longer exist.
func writePages(cliMarkdownDir string, cmd *serpent.Command) {
	// wroteMap indexes file paths to commands.
	wroteMap := make(map[string]*serpent.Command)
	err := genTree(
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
}
