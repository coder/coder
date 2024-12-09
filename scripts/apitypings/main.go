package main

import (
	"fmt"
	"log"

	"github.com/coder/guts"
	"github.com/coder/guts/bindings"
	"github.com/coder/guts/bindings/walk"
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

	referencePackages := map[string]string{
		"github.com/coder/serpent": "Serpent",
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
	fmt.Println(output)
}

func TsMutations(ts *guts.Typescript) {
	ts.ApplyMutations(
		FixSerpentStruct,
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
	)
}

// TypeMappings is all the custom types for codersdk
func TypeMappings(gen *guts.GoParser) error {
	gen.IncludeCustomDeclaration(config.StandardMappings())

	gen.IncludeCustomDeclaration(map[string]guts.TypeOverride{
		"github.com/coder/coder/v2/codersdk.NullTime": config.OverrideNullable(config.OverrideLiteral(bindings.KeywordString)),
	})

	err := gen.IncludeCustom(map[string]string{
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
		"github.com/coder/serpent.HostPort":       "string",
		"encoding/json.RawMessage":                "map[string]string",
	})
	if err != nil {
		return fmt.Errorf("include custom: %w", err)
	}

	return nil
}

// FixSerpentStruct fixes 'serpent.Struct', which defers to the underlying type.
func FixSerpentStruct(gen *guts.Typescript) {
	gen.ForEach(func(key string, originalNode bindings.Node) {
		// replace it with
		// export type SerpentStruct<T extends any> = T
		isInterface, ok := originalNode.(*bindings.Interface)
		if ok && isInterface.Name.Ref() == "SerpentStruct" {
			// TODO: Add a method to add comments here
			gen.ReplaceNode("SerpentStruct", &bindings.Alias{
				Name:      isInterface.Name,
				Modifiers: nil,
				Type: bindings.Reference(bindings.Identifier{
					Name:    "T",
					Package: isInterface.Name.Package,
					Prefix:  "",
				}),
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

type serpentStructVisitor struct {
}

func (s *serpentStructVisitor) Visit(originalNode bindings.Node) walk.Visitor {
	switch node := originalNode.(type) {
	case *bindings.ReferenceType:
		if node.Name.Name == "Struct" && node.Name.PkgName() == "github.com/coder/serpent" {
			// We always expect an argument
			arg := node.Arguments[0]
			*node = *arg.(*bindings.ReferenceType)
			//originalNode = node.Arguments[0]
		}
	}
	return s
}

func ptr[T any](v T) *T {
	return &v
}
