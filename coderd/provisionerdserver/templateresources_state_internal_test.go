package provisionerdserver

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/provisionersdk"
	sdkproto "github.com/coder/coder/v2/provisionersdk/proto"
)

func templateResourceTestState(outputs, resources string) []byte {
	return []byte(`{"version":4,"terraform_version":"1.11.0","serial":1,"lineage":"test-lineage","outputs":` + outputs + `,"resources":` + resources + `}`)
}

func TestTemplateResourceOutputs(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name    string
		outputs string
		want    string
	}{
		{name: "Empty", outputs: `{}`, want: `{}`},
		{name: "SensitiveIncluded", outputs: `{"secret":{"sensitive":true,"value":"password","type":"string"},"public":{"sensitive":false,"value":"hello","type":"string"}}`, want: `{"public":"hello","secret":"password"}`},
		{name: "OmittedSensitivityIsPublic", outputs: `{"greeting":{"value":"hello","type":"string"}}`, want: `{"greeting":"hello"}`},
		{name: "Structured", outputs: `{"config":{"sensitive":false,"value":{"ids":[1,true,null,"x"]}},"large":{"sensitive":false,"value":9007199254740993}}`, want: `{"config":{"ids":[1,true,null,"x"]},"large":9007199254740993}`},
		{name: "NullValue", outputs: `{"empty":{"sensitive":false,"value":null}}`, want: `{"empty":null}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			outputs, err := templateResourceOutputs(templateResourceTestState(tc.outputs, `[]`), "apply")
			require.NoError(t, err)
			values, err := templateResourceOutputsMetadata(outputs)
			require.NoError(t, err)
			require.Equal(t, tc.want, values)
			var metadata sdkproto.Metadata
			require.NoError(t, populateFoundationOutputMetadata(outputs, &metadata))
			var types map[string]provisionersdk.FoundationOutputMetadata
			require.NoError(t, json.Unmarshal([]byte(metadata.FoundationOutputMetadata), &types))
			if tc.name == "SensitiveIncluded" {
				require.True(t, types["secret"].Sensitive)
				require.Equal(t, `"string"`, string(types["secret"].Type))
				require.False(t, types["public"].Sensitive)
			}
		})
	}
}

func TestTemplateResourceOutputsTerraformV4Fixture(t *testing.T) {
	t.Parallel()
	// Terraform's raw-state output objects omit sensitive:false, unlike
	// terraform output -json. This matches the credential-free demo root.
	state := []byte(`{
  "version": 4,
  "terraform_version": "1.9.8",
  "serial": 1,
  "lineage": "28d4ef44-f605-49ab-a2f4-61f705094090",
  "outputs": {
    "greeting": {"value": "Hello from shared resources", "type": "string"},
    "namespace": {"value": "workspaces", "type": "string"},
    "structured": {
      "value": {"enabled": true, "ports": [443, 3100]},
      "type": ["object", {"enabled": "bool", "ports": ["tuple", ["number", "number"]]}]
    },
    "private_setting": {"value": "demo-only-not-a-real-secret", "type": "string", "sensitive": true}
  },
  "resources": [{
    "mode": "managed",
    "type": "terraform_data",
    "name": "settings",
    "provider": "provider[\"terraform.io/builtin/terraform\"]",
    "instances": [{
      "schema_version": 0,
      "attributes": {
        "id": "6d58c7b1-9e22-4b9c-ae3e-64ca52c9a945",
        "input": {
          "value": {"greeting": "Hello from shared resources", "namespace": "workspaces"},
          "type": ["object", {"greeting": "string", "namespace": "string"}]
        },
        "output": {
          "value": {"greeting": "Hello from shared resources", "namespace": "workspaces"},
          "type": ["object", {"greeting": "string", "namespace": "string"}]
        },
        "triggers_replace": null
      },
      "sensitive_attributes": []
    }]
  }],
  "check_results": null
}`)
	outputs, err := templateResourceOutputs(state, "apply")
	require.NoError(t, err)
	snapshot, err := provisionersdk.DecodeFoundationOutputSnapshot(outputs)
	require.NoError(t, err)
	require.Equal(t, `"Hello from shared resources"`, string(snapshot.Values["greeting"]))
	require.True(t, snapshot.Metadata["private_setting"].Sensitive)
	require.Equal(t, `"demo-only-not-a-real-secret"`, string(snapshot.Values["private_setting"]))
	require.JSONEq(t, `["object", {"enabled": "bool", "ports": ["tuple", ["number", "number"]]}]`, string(snapshot.Metadata["structured"].Type))
}

func TestTemplateResourceOutputsSize(t *testing.T) {
	t.Parallel()
	for _, size := range []int{maxTemplateResourceOutputsSize - len(`{"x":""}`), maxTemplateResourceOutputsSize - len(`{"x":""}`) + 1} {
		value, err := json.Marshal(strings.Repeat("x", size))
		require.NoError(t, err)
		outputs, err := templateResourceOutputs(templateResourceTestState(`{"x":{"sensitive":false,"value":`+string(value)+`}}`, `[]`), "apply")
		if size+len(`{"x":""}`) > maxTemplateResourceOutputsSize {
			require.EqualError(t, err, "shared resource outputs exceed the 64 KiB limit")
		} else {
			require.NoError(t, err)
			values, err := templateResourceOutputsMetadata(outputs)
			require.NoError(t, err)
			require.Len(t, values, maxTemplateResourceOutputsSize)
		}
	}
	// Sensitive values use the same private environment payload budget.
	value, err := json.Marshal(strings.Repeat("x", maxTemplateResourceOutputsSize+1))
	require.NoError(t, err)
	_, err = templateResourceOutputs(templateResourceTestState(`{"x":{"sensitive":true,"value":`+string(value)+`}}`, `[]`), "apply")
	require.EqualError(t, err, "shared resource outputs exceed the 64 KiB limit")
}

func TestTemplateResourceOutputsMetadata(t *testing.T) {
	t.Parallel()
	metadata, err := templateResourceOutputsMetadata(json.RawMessage(` { "config" : { "id" : 9007199254740993 } } `))
	require.NoError(t, err)
	require.Equal(t, `{"config":{"id":9007199254740993}}`, metadata)
	metadata, err = templateResourceOutputsMetadata(nil)
	require.NoError(t, err)
	require.Equal(t, "{}", metadata)
	_, err = templateResourceOutputsMetadata(json.RawMessage(`[]`))
	require.Error(t, err)
	encoded, err := json.Marshal(map[string]string{"x": strings.Repeat("x", maxTemplateResourceOutputsSize)})
	require.NoError(t, err)
	_, err = templateResourceOutputsMetadata(encoded)
	require.EqualError(t, err, "shared resource outputs exceed the 64 KiB limit")
}

func TestTemplateResourceStateValidation(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name    string
		state   []byte
		action  string
		wantErr string
	}{
		{name: "Missing", action: "apply", wantErr: "decode"},
		{name: "PlanNotState", action: "apply", state: []byte(`{"format_version":"1.2","planned_values":{}}`), wantErr: "complete Terraform v4"},
		{name: "Null", action: "apply", state: []byte(`null`), wantErr: "complete Terraform v4"},
		{name: "MissingResources", action: "apply", state: templateResourceTestState(`{}`, `null`), wantErr: "complete Terraform v4"},
		{name: "MissingOutputValue", action: "apply", state: templateResourceTestState(`{"x":{"sensitive":false}}`, `[]`), wantErr: "missing its value"},
		{name: "MalformedSensitivity", action: "apply", state: templateResourceTestState(`{"x":{"sensitive":"false","value":"secret"}}`, `[]`), wantErr: "decode"},
		{name: "ManagedApply", action: "apply", state: templateResourceTestState(`{}`, `[{"mode":"managed","instances":[{}]}]`)},
		{name: "ManagedDestroy", state: templateResourceTestState(`{}`, `[{"mode":"managed","instances":[{}]}]`), action: "destroy", wantErr: "managed instances"},
		{name: "MissingInstancesDestroy", state: templateResourceTestState(`{}`, `[{"mode":"managed"}]`), action: "destroy", wantErr: "missing its instance list"},
		{name: "NullInstancesDestroy", state: templateResourceTestState(`{}`, `[{"mode":"managed","instances":null}]`), action: "destroy", wantErr: "missing its instance list"},
		{name: "DeposedDestroy", state: templateResourceTestState(`{}`, `[{"mode":"managed","instances":[{"deposed":"abc"}]}]`), action: "destroy", wantErr: "managed instances"},
		{name: "DataDestroy", state: templateResourceTestState(`{}`, `[{"mode":"data","instances":[{}]}]`), action: "destroy"},
		{name: "EmptyManagedDestroy", state: templateResourceTestState(`{}`, `[{"mode":"managed","instances":[]}]`), action: "destroy"},
		{name: "UnknownMode", state: templateResourceTestState(`{}`, `[{"mode":"unexpected","instances":[]}]`), action: "destroy", wantErr: "invalid resource mode"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := templateResourceOutputs(tc.state, tc.action)
			if tc.wantErr != "" {
				require.ErrorContains(t, err, tc.wantErr)
			} else {
				require.NoError(t, err)
			}
		})
	}
}
