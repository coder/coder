package coderdtest_test

import (
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
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

// TestSwaggerParametersValid guards against parameter keywords that
// Swagger 2.0 rejects: "example" on a non-body parameter, and "format" on an
// array parameter, which swag applies to the array instead of its items.
func TestSwaggerParametersValid(t *testing.T) {
	t.Parallel()

	var swagger struct {
		Paths map[string]map[string]struct {
			Parameters []struct {
				Name    string `json:"name"`
				In      string `json:"in"`
				Type    string `json:"type"`
				Format  string `json:"format"`
				Example any    `json:"example"`
			} `json:"parameters"`
		} `json:"paths"`
	}
	require.NoError(t, json.Unmarshal([]byte(apidoc.SwaggerInfo.ReadDoc()), &swagger))
	require.NotEmpty(t, swagger.Paths)

	for path, operations := range swagger.Paths {
		for method, operation := range operations {
			for _, parameter := range operation.Parameters {
				if parameter.In != "body" {
					assert.Nil(t, parameter.Example, "%s %s: %s parameter %q must not have an example; use extensions(x-example=...)", method, path, parameter.In, parameter.Name)
				}
				if parameter.Type == "array" {
					assert.Empty(t, parameter.Format, "%s %s: array parameter %q must not have a format", method, path, parameter.Name)
				}
			}
		}
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
