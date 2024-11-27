package main

import (
	"fmt"
	"log"

	"github.com/coder/guts"
	"github.com/coder/guts/config"

	// Must import the packages we are trying to convert
	// And include the ones we are referencing
	//_ "github.com/coder/coder/coderd/healthcheck/health"
	//_ "github.com/coder/coder/codersdk/health"
	_ "github.com/coder/coder/v2/codersdk"
	_ "github.com/coder/serpent"
)

func main() {
	gen, err := guts.NewGolangParser()
	if err != nil {
		log.Fatalf("new convert: %v", err)
	}

	generateDirectories := []string{
		"github.com/coder/coder/v2/codersdk",
		"github.com/coder/coder/v2/codersdk/health",
	}
	for _, dir := range generateDirectories {
		err = gen.Include(dir, true)
		if err != nil {
			log.Fatalf("include generate package %q: %v", dir, err)
		}
	}

	referencePackages := []string{
		"github.com/coder/serpent",
		"github.com/coder/coder/v2/coderd/healthcheck/health",
	}
	for _, pkg := range referencePackages {
		err = gen.Include(pkg, false)
		if err != nil {
			log.Fatalf("include reference package %q: %v", pkg, err)
		}
	}

	err = TypeMappings(gen)
	if err != nil {
		log.Fatalf("type mappings: %v", err)
	}

	ts, err := gen.ToTypescript()
	if err != nil {
		log.Fatalf("to typescript: %v", err)
	}

	TsMutations(ts)

	output, err := ts.Serialize()
	if err != nil {
		log.Fatalf("serialize: %v", err)
	}
	fmt.Println(output)
}

func TsMutations(ts *guts.Typescript) {
	ts.ApplyMutations(
		// Enum list generator
		config.EnumLists,
		// Export all top level types
		config.ExportTypes,
		// Readonly interface fields
		config.ReadOnly,
	)
}

// TypeMappings is all the custom types for codersdk
func TypeMappings(gen *guts.GoParser) error {
	gen.IncludeCustomDeclaration(config.StandardMappings())

	err := gen.IncludeCustom(map[string]string{
		"github.com/coder/coder/v2/codersdk.NullTime": "string",
		// Serpent fields
		"github.com/coder/serpent.Regexp":         "string",
		"github.com/coder/serpent.StringArray":    "string",
		"github.com/coder/serpent.String":         "string",
		"github.com/coder/serpent.YAMLConfigPath": "string",
		"github.com/coder/serpent.Strings":        "[]string",
		"github.com/coder/serpent.Int64":          "int64",
		"github.com/coder/serpent.Bool":           "bool",
		"github.com/coder/serpent.Duration":       "int64",
		"github.com/coder/serpent.URL":            "string",
		"encoding/json.RawMessage":                "map[string]string",
	})
	if err != nil {
		return fmt.Errorf("include custom: %w", err)
	}

	return nil
}
