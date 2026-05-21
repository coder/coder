package main

import (
	"fmt"
	"log"
	"reflect"
	"strings"

	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/guts"
	"github.com/coder/guts/bindings"
	"github.com/coder/guts/config"
)

func main() {
	gen, err := guts.NewGolangParser()
	if err != nil {
		log.Fatalf("new convert: %v", err)
	}

	// Include golang comments to typescript output.
	gen.PreserveComments()

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

	TSMutations(ts)

	output, err := ts.Serialize()
	if err != nil {
		log.Fatalf("serialize: %v", err)
	}
	_, _ = fmt.Println(output)
}

func TSMutations(ts *guts.Typescript) {
	ts.ApplyMutations(
		// TODO: Remove 'NotNullMaps'. This is hiding potential bugs
		//   of referencing maps that are actually null.
		config.NotNullMaps,
		FixSerpentStruct,
		DiscriminatedChatMessagePart,
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
		// decimal.Decimal preserves exact pricing precision (e.g. $3.50 per
		// million tokens) and serializes as a JSON string to avoid
		// floating-point loss in transit.
		"github.com/shopspring/decimal.Decimal": "string",
	})
	if err != nil {
		return xerrors.Errorf("include custom: %w", err)
	}

	return nil
}

// DiscriminatedChatMessagePart splits the flat ChatMessagePart
// interface into a discriminated union of per-type sub-interfaces.
// Each sub-interface narrows the `type` field to a string literal
// and includes only the fields relevant to that part type.
//
// Variant membership is declared via `variants` struct tags on
// ChatMessagePart fields in codersdk/chats.go. This function
// reads those tags via reflect and builds the union from them.
func DiscriminatedChatMessagePart(ts *guts.Typescript) {
	node, ok := ts.Node("ChatMessagePart")
	if !ok {
		return
	}
	iface, ok := node.(*bindings.Interface)
	if !ok {
		return
	}

	// Build a lookup from field name to its PropertySignature so
	// we can copy type information from the original interface.
	fieldMap := make(map[string]*bindings.PropertySignature, len(iface.Fields))
	for _, f := range iface.Fields {
		fieldMap[f.Name] = f
	}

	// copyField copies a field from the original interface into a
	// sub-interface, setting QuestionToken based on whether the
	// field is required for that variant.
	copyField := func(name string, required bool) *bindings.PropertySignature {
		orig, exists := fieldMap[name]
		if !exists {
			return nil
		}
		return &bindings.PropertySignature{
			Name:            orig.Name,
			Modifiers:       orig.Modifiers,
			QuestionToken:   !required,
			Type:            orig.Type,
			SupportComments: orig.SupportComments,
		}
	}

	variants := parseVariantTags()
	unionMembers := make([]bindings.ExpressionType, 0, len(variants))

	for _, v := range variants {
		fields := make([]*bindings.PropertySignature, 0, 1+len(v.required)+len(v.optional))

		// Discriminant field: type narrowed to a string literal.
		fields = append(fields, &bindings.PropertySignature{
			Name: "type",
			Type: &bindings.LiteralType{Value: string(v.typeLiteral)},
		})

		for _, name := range v.required {
			if f := copyField(name, true); f != nil {
				fields = append(fields, f)
			}
		}
		for _, name := range v.optional {
			if f := copyField(name, false); f != nil {
				fields = append(fields, f)
			}
		}

		tsName := chatMessagePartTSName(v.typeLiteral)
		subIface := &bindings.Interface{
			Name: bindings.Identifier{
				Name:    tsName,
				Package: iface.Name.Package,
				Prefix:  iface.Name.Prefix,
			},
			Fields: fields,
			Source: iface.Source,
		}

		// Inject the sub-interface as a new top-level type.
		if err := ts.SetNode(tsName, subIface); err != nil {
			panic(fmt.Sprintf("ChatMessagePart variant %q: %v", v.typeLiteral, err))
		}

		unionMembers = append(unionMembers, bindings.Reference(bindings.Identifier{
			Name:    tsName,
			Package: iface.Name.Package,
			Prefix:  iface.Name.Prefix,
		}))
	}

	// Replace the original flat interface with a union alias.
	ts.ReplaceNode("ChatMessagePart", &bindings.Alias{
		Name:            iface.Name,
		Modifiers:       iface.Modifiers,
		Type:            bindings.Union(unionMembers...),
		SupportComments: iface.SupportComments,
		Source:          iface.Source,
	})
}

// chatPartVariant holds the parsed variant info for one part type.
type chatPartVariant struct {
	typeLiteral codersdk.ChatMessagePartType
	required    []string // JSON field names
	optional    []string // JSON field names
}

// parseVariantTags reads `variants` struct tags from ChatMessagePart
// and returns the per-type field sets using JSON tag names. Variants
// are returned in AllChatMessagePartTypes order for stable codegen.
func parseVariantTags() []chatPartVariant {
	t := reflect.TypeFor[codersdk.ChatMessagePart]()

	type fieldSets struct {
		required []string
		optional []string
	}
	byType := make(map[codersdk.ChatMessagePartType]*fieldSets)

	for i := range t.NumField() {
		f := t.Field(i)
		varTag := f.Tag.Get("variants")
		if varTag == "" {
			continue
		}
		jsonName, _, _ := strings.Cut(f.Tag.Get("json"), ",")
		for entry := range strings.SplitSeq(varTag, ",") {
			isOptional := strings.HasSuffix(entry, "?")
			typeLit := codersdk.ChatMessagePartType(strings.TrimSuffix(entry, "?"))
			if byType[typeLit] == nil {
				byType[typeLit] = &fieldSets{}
			}
			if isOptional {
				byType[typeLit].optional = append(byType[typeLit].optional, jsonName)
			} else {
				byType[typeLit].required = append(byType[typeLit].required, jsonName)
			}
		}
	}

	result := make([]chatPartVariant, 0, len(byType))
	for _, pt := range codersdk.AllChatMessagePartTypes() {
		if fs, ok := byType[pt]; ok {
			result = append(result, chatPartVariant{
				typeLiteral: pt,
				required:    fs.required,
				optional:    fs.optional,
			})
		}
	}
	return result
}

// chatMessagePartTSName derives a TypeScript interface name from
// a ChatMessagePartType literal. "tool-call" → "ChatToolCallPart".
func chatMessagePartTSName(t codersdk.ChatMessagePartType) string {
	words := strings.Split(string(t), "-")
	for i, w := range words {
		if len(w) > 0 {
			words[i] = strings.ToUpper(w[:1]) + w[1:]
		}
	}
	return "Chat" + strings.Join(words, "") + "Part"
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
