package main

import (
	"fmt"
	"log"

	"golang.org/x/xerrors"

	"github.com/coder/guts"
	"github.com/coder/guts/bindings"
	"github.com/coder/guts/config"
)

func main() {
	gen, err := guts.NewGolangParser()
	if err != nil {
		log.Fatalf("new convert: %v", err)
	}

	generateDirectories := map[string]string{
		"github.com/coder/coder/v2/codersdk":                  "",
		"github.com/coder/coder/v2/coderd/healthcheck/health": "Health",
		"github.com/coder/coder/v2/codersdk/healthsdk":        "",
	}
	for dir, prefix := range generateDirectories {
		err = gen.IncludeGenerateWithPrefix(dir, prefix)
		if err != nil {
			log.Fatalf("include generate package %q: %v", dir, err)
		}
	}

	// Serpent has some types referenced in the codersdk.
	// We want the referenced types generated.
	referencePackages := map[string]string{
		"github.com/coder/preview/types": "Preview",
		"github.com/coder/serpent":       "Serpent",
		"tailscale.com/derp":             "",
		// Conflicting name "DERPRegion"
		"tailscale.com/tailcfg":      "Tail",
		"tailscale.com/net/netcheck": "Netcheck",
	}
	for pkg, prefix := range referencePackages {
		err = gen.IncludeReference(pkg, prefix)
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
	_, _ = fmt.Println(output)
}

func TsMutations(ts *guts.Typescript) {
	ts.ApplyMutations(
		FixSerpentStruct,
		// TODO: Remove 'NotNullMaps'. This is hiding potential bugs
		//   of referencing maps that are actually null.
		config.NotNullMaps,
		// Prefer enums as types
		config.EnumAsTypes,
		// Enum list generator
		config.EnumLists,
		// Export all top level types
		config.ExportTypes,
		// Readonly interface fields
		config.ReadOnly,
		// Add ignore linter comments
		config.BiomeLintIgnoreAnyTypeParameters,
		// Omitempty + null is just '?' in golang json marshal
		// number?: number | null --> number?: number
		config.SimplifyOmitEmpty,
		// TsType: (string | null)[] --> (string)[]
		config.NullUnionSlices,
	)
}

// TypeMappings is all the custom types for codersdk
func TypeMappings(gen *guts.GoParser) error {
	gen.IncludeCustomDeclaration(config.StandardMappings())

	gen.IncludeCustomDeclaration(map[string]guts.TypeOverride{
		"github.com/coder/coder/v2/codersdk.NullTime": config.OverrideNullable(config.OverrideLiteral(bindings.KeywordString)),
		// opt.Bool can return 'null' if unset
		"tailscale.com/types/opt.Bool": config.OverrideNullable(config.OverrideLiteral(bindings.KeywordBoolean)),
		// hcl diagnostics should be cast to `preview.FriendlyDiagnostic`
		"github.com/hashicorp/hcl/v2.Diagnostic": func() bindings.ExpressionType {
			return bindings.Reference(bindings.Identifier{
				Name:    "FriendlyDiagnostic",
				Package: nil,
				Prefix:  "",
			})
		},
		"github.com/coder/preview/types.HCLString": func() bindings.ExpressionType {
			return bindings.Reference(bindings.Identifier{
				Name:    "NullHCLString",
				Package: nil,
				Prefix:  "",
			})
		},
	})

	err := gen.IncludeCustom(map[string]string{
		// Serpent fields should be converted to their primitive types
		"github.com/coder/serpent.Regexp":         "string",
		"github.com/coder/serpent.StringArray":    "string",
		"github.com/coder/serpent.String":         "string",
		"github.com/coder/serpent.YAMLConfigPath": "string",
		"github.com/coder/serpent.Strings":        "[]string",
		"github.com/coder/serpent.Int64":          "int64",
		"github.com/coder/serpent.Bool":           "bool",
		"github.com/coder/serpent.Duration":       "int64",
		"github.com/coder/serpent.URL":            "string",
		"github.com/coder/serpent.HostPort":       "string",
		"encoding/json.RawMessage":                "map[string]string",
	})
	if err != nil {
		return xerrors.Errorf("include custom: %w", err)
	}

	return nil
}

// FixSerpentStruct fixes 'serpent.Struct'.
// 'serpent.Struct' overrides the json.Marshal to use the underlying type,
// so the typescript type should be the underlying type.
func FixSerpentStruct(gen *guts.Typescript) {
	gen.ForEach(func(_ string, originalNode bindings.Node) {
		isInterface, ok := originalNode.(*bindings.Interface)
		if ok && isInterface.Name.Ref() == "SerpentStruct" {
			// replace it with
			// export type SerpentStruct<T> = T
			gen.ReplaceNode("SerpentStruct", &bindings.Alias{
				Name:      isInterface.Name,
				Modifiers: nil,
				// The RHS expression is just 'T'
				Type: bindings.Reference(bindings.Identifier{
					Name:    "T",
					Package: isInterface.Name.Package,
					Prefix:  "",
				}),
				// Generic type parameters, T can be anything.
				// Do not provide it a type, as it 'extends any'
				Parameters: []*bindings.TypeParameter{
					{
						Name: bindings.Identifier{
							Name:    "T",
							Package: isInterface.Name.Package,
							Prefix:  "",
						},
						Modifiers:   nil,
						Type:        nil,
						DefaultType: nil,
					},
				},
				Source: isInterface.Source,
			})
		}
	})
}
