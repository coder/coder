package coderdtest_test

import (
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/apidoc"
	"github.com/coder/coder/v2/coderd/coderdtest"
)

func TestEndpointsDocumented(t *testing.T) {
	t.Parallel()

	swaggerComments, err := coderdtest.ParseSwaggerComments("..", "../workspaceconnwatcher")
	require.NoError(t, err, "can't parse swagger comments")
	require.NotEmpty(t, swaggerComments, "swagger comments must be present")

	_, _, api := coderdtest.NewWithAPI(t, nil)
	coderdtest.VerifySwaggerDefinitions(t, api.APIHandler, swaggerComments, coderdtest.WithSwaggerRoutePrefix("/api/v2"))
}

func TestChatModelPathParametersFormatted(t *testing.T) {
	t.Parallel()

	var swagger struct {
		Paths map[string]map[string]struct {
			Parameters []struct {
				Name   string `json:"name"`
				In     string `json:"in"`
				Format string `json:"format"`
			} `json:"parameters"`
		} `json:"paths"`
	}
	require.NoError(t, json.Unmarshal([]byte(apidoc.SwaggerInfo.ReadDoc()), &swagger))

	operations := swagger.Paths["/api/v2/organizations/{organization}/chats/models/{model}"]
	for _, method := range []string{"get", "patch", "delete"} {
		t.Run(method, func(t *testing.T) {
			t.Parallel()

			for _, parameter := range operations[method].Parameters {
				if parameter.Name == "model" && parameter.In == "path" {
					require.Equal(t, "uuid", parameter.Format)
					return
				}
			}
			require.Fail(t, "model path parameter not found")
		})
	}
}

func TestSDKFieldsFormatted(t *testing.T) {
	t.Parallel()

	fileSet := token.NewFileSet()
	nodes, err := parser.ParseDir(fileSet, "../../codersdk", nil, parser.ParseComments)
	require.NoError(t, err, "parser.ParseDir failed")

	for _, node := range nodes {
		ast.Inspect(node, func(n ast.Node) bool {
			typeSpec, ok := n.(*ast.TypeSpec)
			if !ok {
				return true
			}
			structureName := typeSpec.Name

			structType, ok := typeSpec.Type.(*ast.StructType)
			if !ok {
				return true // not a structure
			}

			for _, field := range structType.Fields.List {
				selectorExpr, ok := field.Type.(*ast.SelectorExpr)
				if !ok {
					continue // rather a basic, or primitive
				}

				if field.Tag == nil || !strings.Contains(field.Tag.Value, `json:"`) {
					continue // not a JSON property
				}

				switch selectorExpr.Sel.Name {
				case "UUID":
					assert.Contains(t, field.Tag.Value, `format:"uuid"`, `Swagger formatting requires to annotate the field with - format:"uuid". Location: %s/%s`, structureName, field.Names)
				case "Time":
					assert.Contains(t, field.Tag.Value, `format:"date-time"`, `Swagger formatting requires to annotate the field with - format:"date-time". Location: %s/%s`, structureName, field.Names)
				}
			}
			return true
		})
	}
}

// TestSwaggerEnumsNotNarrowed guards against an `enums:"..."` struct tag
// narrowing a named enum type, which makes the spec list fewer values than the
// API can return, and against duplicate values in an enum list.
func TestSwaggerEnumsNotNarrowed(t *testing.T) {
	t.Parallel()

	// Properties that intentionally accept or document only a subset of the
	// values of their named type, usually because a request may set only some
	// of them. Keep this list as small as possible.
	allowedSubsets := map[string]bool{
		"codersdk.APIKey.scope":                                    true,
		"codersdk.ChatGroup.role":                                  true,
		"codersdk.ChatProjectGroup.role":                           true,
		"codersdk.ChatProjectUser.role":                            true,
		"codersdk.ChatUser.role":                                   true,
		"codersdk.CreateWorkspaceBuildOnSuccessRequest.transition": true,
		"codersdk.MCPServerConfigGroup.role":                       true,
		"codersdk.MCPServerConfigUser.role":                        true,
		"codersdk.OrganizationSkillGroup.role":                     true,
		"codersdk.OrganizationSkillUser.role":                      true,
		"codersdk.TemplateGroup.role":                              true,
		"codersdk.TemplateUser.role":                               true,
		"codersdk.UpdateUserAppearanceSettingsRequest.theme_mode":  true,
		"codersdk.WorkspaceGroup.role":                             true,
		"codersdk.WorkspaceUser.role":                              true,
	}

	type schema struct {
		Ref   string `json:"$ref"`
		AllOf []struct {
			Ref string `json:"$ref"`
		} `json:"allOf"`
		Enum  []any `json:"enum"`
		Items *struct {
			Enum []any `json:"enum"`
		} `json:"items"`
	}
	var swagger struct {
		Definitions map[string]struct {
			Enum       []any             `json:"enum"`
			Properties map[string]schema `json:"properties"`
		} `json:"definitions"`
		Paths map[string]map[string]struct {
			Parameters []struct {
				Name  string `json:"name"`
				Enum  []any  `json:"enum"`
				Items *struct {
					Enum []any `json:"enum"`
				} `json:"items"`
			} `json:"parameters"`
		} `json:"paths"`
	}
	require.NoError(t, json.Unmarshal([]byte(apidoc.SwaggerInfo.ReadDoc()), &swagger))

	seenSubsets := map[string]bool{}
	for defName, def := range swagger.Definitions {
		for propName, prop := range def.Properties {
			ref := prop.Ref
			if ref == "" && len(prop.AllOf) > 0 {
				ref = prop.AllOf[0].Ref
			}
			if ref == "" || len(prop.Enum) == 0 {
				continue
			}
			key := defName + "." + propName
			named := swagger.Definitions[strings.TrimPrefix(ref, "#/definitions/")].Enum
			if slices.Equal(enumStrings(prop.Enum), enumStrings(named)) {
				continue
			}
			seenSubsets[key] = true
			assert.Truef(t, allowedSubsets[key], "%s lists %v but its type %s lists %v; remove the enums tag so the spec lists every value, or add it to allowedSubsets if the subset is intentional", key, prop.Enum, ref, named)
		}
	}
	for key := range allowedSubsets {
		assert.Truef(t, seenSubsets[key], "%s is in allowedSubsets but is no longer a subset of its type; remove it", key)
	}

	for route, methods := range swagger.Paths {
		for method, op := range methods {
			for _, param := range op.Parameters {
				values := param.Enum
				if param.Items != nil {
					values = append(values, param.Items.Enum...)
				}
				seen := map[string]bool{}
				for _, v := range enumStrings(values) {
					assert.Falsef(t, seen[v], "%s %s parameter %q lists enum value %q more than once", method, route, param.Name, v)
					seen[v] = true
				}
			}
		}
	}
}

// enumStrings returns the sorted string form of enum values, which may be
// numbers in the spec.
func enumStrings(values []any) []string {
	out := make([]string, 0, len(values))
	for _, v := range values {
		out = append(out, fmt.Sprint(v))
	}
	slices.Sort(out)
	return out
}
