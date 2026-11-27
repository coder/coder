// Command docsmanifestgen compiles the YAML sidebar sources under
// docs/manifest into docs/manifest.json, formats those sources, checks
// them, and writes the REST API sidebar fragment from the Swagger document.
//
// Usage:
//
//	docsmanifestgen build    [-docs docs] [-out docs/manifest.json]
//	docsmanifestgen fmt      [-docs docs]
//	docsmanifestgen check    [-docs docs]
//	docsmanifestgen rest-api [-docs docs] [-swagger coderd/apidoc/swagger.json] [-out fragment]
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
		return xerrors.New("usage: docsmanifestgen <build|fmt|check|rest-api> [flags]")
	}
	fs := flag.NewFlagSet("docsmanifestgen "+args[0], flag.ContinueOnError)
	docsDir := fs.String("docs", "docs", "Docs directory. Sources are read from its manifest subdirectory.")
	out := fs.String("out", "", "Output path. For build, defaults to manifest.json in the docs directory; for rest-api, to the fragment the REST API route names in children_from.")
	swagger := fs.String("swagger", filepath.Join("coderd", "apidoc", "swagger.json"), "Swagger document that rest-api reads the REST API sections from.")
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
		} else {
			errs = append(errs, repeatedStates(sourcesDir)...)
		}
		unformatted, err := unformattedSources(sourcesDir)
		if err != nil {
			errs = append(errs, err)
		}
		for _, f := range unformatted {
			errs = append(errs, xerrors.Errorf("%s: not formatted; run make fmt/docs-manifest", f.rel))
		}
		return errors.Join(errs...)
	case "rest-api":
		return writeRESTAPIFragment(sourcesDir, *swagger, *out)
	default:
		return xerrors.Errorf("unknown subcommand %q: want build, fmt, check, or rest-api", args[0])
	}
}

// writeRESTAPIFragment writes the REST API sidebar entries for the Swagger
// document at swaggerPath to out, or, when out is empty, to the fragment the
// REST API route in the sources names in children_from.
func writeRESTAPIFragment(sourcesDir, swaggerPath, out string) error {
	swagger, err := os.ReadFile(swaggerPath)
	if err != nil {
		return xerrors.Errorf("read the Swagger document: %w", err)
	}
	routes, err := docgenenv.RESTAPIRoutes(swagger)
	if err != nil {
		return xerrors.Errorf("%s: %w", swaggerPath, err)
	}
	if out == "" {
		m, err := docgenenv.LoadManifestSources(sourcesDir)
		if err != nil {
			return err
		}
		restAPI := m.FindRoute("Reference", "REST API")
		if restAPI == nil || restAPI.ChildrenFrom == "" {
			return xerrors.Errorf("the Reference > REST API route in %s must set children_from", sourcesDir)
		}
		out = filepath.Join(sourcesDir, filepath.FromSlash(restAPI.ChildrenFrom))
	}
	return docgenenv.WriteRouteFragment(out, routes)
}
