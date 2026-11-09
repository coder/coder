// Command docsmanifestgen compiles the YAML sidebar sources under
// docs/manifest into docs/manifest.json, formats those sources, and checks
// them.
//
// Usage:
//
//	docsmanifestgen build [-docs docs] [-out docs/manifest.json]
//	docsmanifestgen fmt   [-docs docs]
//	docsmanifestgen check [-docs docs]
package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/scripts/atomicwrite"
	"github.com/coder/coder/v2/scripts/docgenenv"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		return xerrors.New("usage: docsmanifestgen <build|fmt|check> [flags]")
	}
	fs := flag.NewFlagSet("docsmanifestgen "+args[0], flag.ContinueOnError)
	docsDir := fs.String("docs", "docs", "Docs directory. Sources are read from its manifest subdirectory.")
	out := fs.String("out", "", "Output path for build. Defaults to manifest.json in the docs directory.")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	sourcesDir := filepath.Join(*docsDir, docgenenv.ManifestSourcesDir)

	switch args[0] {
	case "build":
		m, err := docgenenv.BuildManifest(sourcesDir, *docsDir)
		if err != nil {
			return err
		}
		b, err := docgenenv.MarshalJSON(m)
		if err != nil {
			return err
		}
		if *out == "" {
			*out = filepath.Join(*docsDir, "manifest.json")
		}
		return atomicwrite.File(*out, b)
	case "fmt":
		return formatSources(sourcesDir)
	case "check":
		var errs []error
		if _, err := docgenenv.BuildManifest(sourcesDir, *docsDir); err != nil {
			errs = append(errs, err)
		}
		unformatted, err := unformattedSources(sourcesDir)
		if err != nil {
			errs = append(errs, err)
		}
		for _, f := range unformatted {
			errs = append(errs, xerrors.Errorf("%s: not formatted; run make fmt/docs-manifest", f.rel))
		}
		return errors.Join(errs...)
	default:
		return xerrors.Errorf("unknown subcommand %q: want build, fmt, or check", args[0])
	}
}
