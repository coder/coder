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

	// Coder Tasks has no swagger annotations because it is withdrawn from the
	// product, so verify against a deployment where its routes are not
	// registered.
	values := coderdtest.DeploymentValues(t)
	values.EnableAITasks = false

	_, _, api := coderdtest.NewWithAPI(t, &coderdtest.Options{DeploymentValues: values})
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
