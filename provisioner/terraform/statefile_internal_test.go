package terraform

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	tfjson "github.com/hashicorp/terraform-json"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/testutil"
)

// testRawState exercises every shape of state file entry Coder may see:
// root and nested module instances (including an intermediate module that
// holds no resources itself), count and for_each indexes, data sources,
// a tainted instance, a deposed object, and an aliased provider.
const testRawState = `{
  "version": 4,
  "terraform_version": "1.16.2",
  "serial": 7,
  "lineage": "8f7a6a2e-1111-2222-3333-444444444444",
  "outputs": {},
  "resources": [
    {
      "mode": "data",
      "type": "coder_workspace",
      "name": "me",
      "provider": "provider[\"registry.terraform.io/coder/coder\"]",
      "instances": [
        {
          "schema_version": 0,
          "attributes": {"id": "ws-1", "name": "dev", "start_count": 1},
          "sensitive_attributes": []
        }
      ]
    },
    {
      "mode": "managed",
      "type": "coder_agent",
      "name": "main",
      "provider": "provider[\"registry.terraform.io/coder/coder\"]",
      "instances": [
        {
          "schema_version": 1,
          "attributes": {"id": "agent-1", "arch": "amd64", "os": "linux", "token": "secret", "env": {"A": "1"}, "startup_script": null},
          "sensitive_attributes": [[{"type":"get_attr","value":"token"}]],
          "dependencies": ["data.coder_workspace.me"]
        }
      ]
    },
    {
      "mode": "managed",
      "type": "docker_container",
      "name": "workspace",
      "provider": "provider[\"registry.terraform.io/kreuzwerker/docker\"].alt",
      "instances": [
        {
          "index_key": 0,
          "schema_version": 2,
          "status": "tainted",
          "attributes": {"id": "c-0", "ports": [{"internal": 80, "external": 8080}]},
          "dependencies": ["coder_agent.main", "data.coder_workspace.me"]
        },
        {
          "index_key": 0,
          "deposed": "abcdef12",
          "schema_version": 2,
          "attributes": {"id": "c-old"}
        }
      ]
    },
    {
      "module": "module.outer[\"a.module.b\"].module.inner",
      "mode": "managed",
      "type": "coder_app",
      "name": "code",
      "provider": "module.outer[\"a.module.b\"].module.inner.provider[\"registry.terraform.io/coder/coder\"]",
      "instances": [
        {
          "index_key": "vscode",
          "schema_version": 0,
          "attributes": {"id": "app-1", "slug": "vscode"}
        },
        {
          "index_key": "web \"ui\"",
          "schema_version": 0,
          "attributes": {"id": "app-2", "slug": "web"}
        }
      ]
    }
  ]
}`

func Test_convertRawState(t *testing.T) {
	t.Parallel()

	state, err := convertRawState([]byte(testRawState))
	require.NoError(t, err)
	require.Equal(t, "1.0", state.FormatVersion)
	require.Equal(t, "1.16.2", state.TerraformVersion)
	require.NotNil(t, state.Values)

	root := state.Values.RootModule
	require.Equal(t, "", root.Address)

	addresses := func(m *tfjson.StateModule) []string {
		out := []string{}
		for _, r := range m.Resources {
			out = append(out, r.Address)
		}
		return out
	}
	// Ordered like terraform show: data sources first, then by type and
	// name, with the deposed object after its current one.
	require.Equal(t, []string{
		"data.coder_workspace.me",
		"coder_agent.main",
		"docker_container.workspace[0]",
		"docker_container.workspace[0]",
	}, addresses(root))

	data := root.Resources[0]
	require.Equal(t, tfjson.DataResourceMode, data.Mode)
	require.Equal(t, float64(1), data.AttributeValues["start_count"])

	agent := root.Resources[1]
	require.Equal(t, tfjson.ManagedResourceMode, agent.Mode)
	require.Equal(t, "coder_agent", agent.Type)
	require.Equal(t, "main", agent.Name)
	require.Nil(t, agent.Index)
	require.Equal(t, "registry.terraform.io/coder/coder", agent.ProviderName)
	require.EqualValues(t, 1, agent.SchemaVersion)
	require.Equal(t, map[string]interface{}{
		"id":             "agent-1",
		"arch":           "amd64",
		"os":             "linux",
		"token":          "secret",
		"env":            map[string]interface{}{"A": "1"},
		"startup_script": nil,
	}, agent.AttributeValues)
	require.Equal(t, []string{"data.coder_workspace.me"}, agent.DependsOn)
	require.False(t, agent.Tainted)

	current, deposed := root.Resources[2], root.Resources[3]
	require.Empty(t, current.DeposedKey)
	require.Equal(t, float64(0), current.Index)
	require.Equal(t, "registry.terraform.io/kreuzwerker/docker", current.ProviderName, "alias is not part of the provider name")
	require.True(t, current.Tainted)
	require.Equal(t, "c-0", current.AttributeValues["id"])
	require.Equal(t, "abcdef12", deposed.DeposedKey)
	require.Equal(t, "c-old", deposed.AttributeValues["id"])

	// module.outer["a.module.b"] holds no resources but is still created as
	// the parent of module.inner. The key deliberately contains ".module."
	// to make sure the module path is not split naively.
	require.Len(t, root.ChildModules, 1)
	outer := root.ChildModules[0]
	require.Equal(t, `module.outer["a.module.b"]`, outer.Address)
	require.Empty(t, outer.Resources)
	require.Len(t, outer.ChildModules, 1)
	inner := outer.ChildModules[0]
	require.Equal(t, `module.outer["a.module.b"].module.inner`, inner.Address)
	require.Equal(t, []string{
		`module.outer["a.module.b"].module.inner.coder_app.code["vscode"]`,
		`module.outer["a.module.b"].module.inner.coder_app.code["web \"ui\""]`,
	}, addresses(inner))
	require.Equal(t, "vscode", inner.Resources[0].Index)
	require.Equal(t, "registry.terraform.io/coder/coder", inner.Resources[0].ProviderName)
}

func Test_convertRawState_Rejects(t *testing.T) {
	t.Parallel()

	t.Run("EmptyState", func(t *testing.T) {
		t.Parallel()
		state, err := convertRawState([]byte(`{"version":4,"terraform_version":"1.16.2","resources":[]}`))
		require.NoError(t, err)
		require.Nil(t, state.Values, "terraform show omits values for an empty state")
	})

	t.Run("OldVersion", func(t *testing.T) {
		t.Parallel()
		_, err := convertRawState([]byte(`{"version":3,"modules":[]}`))
		require.ErrorContains(t, err, "unsupported state file version")
	})

	t.Run("FlatmapAttributes", func(t *testing.T) {
		t.Parallel()
		_, err := convertRawState([]byte(`{"version":4,"resources":[{"mode":"managed","type":"a","name":"b","provider":"provider[\"x/y/z\"]","instances":[{"attributes_flat":{"id":"1"}}]}]}`))
		require.ErrorContains(t, err, "legacy flatmap")
	})

	t.Run("MissingFile", func(t *testing.T) {
		t.Parallel()
		state, err := readStateFile(filepath.Join(t.TempDir(), "terraform.tfstate"))
		require.NoError(t, err)
		require.Nil(t, state.Values)
	})
}

func Test_parentModule(t *testing.T) {
	t.Parallel()
	for input, want := range map[string]string{
		"module.a":                                "",
		"module.a.module.b":                       "module.a",
		`module.a["x.module.y"].module.b`:         `module.a["x.module.y"]`,
		`module.a[0].module.b["k"].module.c`:      `module.a[0].module.b["k"]`,
		`module.a["esc \".module.\" q"].module.b`: `module.a["esc \".module.\" q"]`,
	} {
		require.Equal(t, want, parentModule(input), input)
	}
}

// Test_readStateFile_MatchesTerraformShow applies a configuration with real
// Terraform and checks that reading the state file yields the same modules
// and resources as `terraform show -json`. It uses the coder provider,
// whose resources are the ones Coder decodes attribute values from.
func Test_readStateFile_MatchesTerraformShow(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitLong)

	binary, err := systemBinary(ctx)
	if err != nil {
		var installErr error
		path, installErr := Install(ctx, slogtest.Make(t, nil), false, t.TempDir(), TerraformVersion, "")
		require.NoError(t, installErr, "no usable terraform binary: %v", err)
		binary = &systemBinaryDetails{absolutePath: path}
	}

	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "child", "grandchild"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "main.tf"), []byte(`
terraform {
  required_providers {
    coder = {
      source  = "coder/coder"
      version = "2.15.0"
    }
  }
}
data "coder_workspace" "me" {}
resource "coder_agent" "main" {
  count = 2
  arch  = "amd64"
  os    = "linux"
  env   = { INDEX = tostring(count.index) }
  metadata {
    key      = "load"
    script   = "uptime"
    interval = 10
  }
}
resource "coder_app" "apps" {
  for_each     = toset(["code", "web ui"])
  agent_id     = coder_agent.main[0].id
  slug         = replace(each.key, " ", "-")
  display_name = each.key
}
module "child" {
  source   = "./child"
  agent_id = coder_agent.main[1].id
}
`), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "child", "main.tf"), []byte(`
terraform {
  required_providers {
    coder = { source = "coder/coder" }
  }
}
variable "agent_id" { type = string }
module "grandchild" {
  for_each = toset(["one"])
  source   = "./grandchild"
  agent_id = var.agent_id
}
`), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "child", "grandchild", "main.tf"), []byte(`
terraform {
  required_providers {
    coder = { source = "coder/coder" }
  }
}
variable "agent_id" { type = string }
resource "coder_script" "deep" {
  agent_id     = var.agent_id
  display_name = "deep"
  script       = "echo deep"
  run_on_start = true
}
`), 0o600))

	run := func(args ...string) []byte {
		//nolint:gosec // Test-controlled terraform binary and arguments.
		cmd := exec.CommandContext(ctx, binary.absolutePath, args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), "TF_IN_AUTOMATION=1", "CHECKPOINT_DISABLE=1")
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		out, err := cmd.Output()
		require.NoError(t, err, "%s: %s", args, stderr.String())
		return out
	}
	run("init", "-no-color", "-input=false")
	run("apply", "-no-color", "-input=false", "-auto-approve")

	var want tfjson.State
	require.NoError(t, json.Unmarshal(run("show", "-json", "-no-color"), &want))

	got, err := readStateFile(filepath.Join(dir, "terraform.tfstate"))
	require.NoError(t, err)

	require.Equal(t, want.FormatVersion, got.FormatVersion)
	require.Equal(t, want.TerraformVersion, got.TerraformVersion)
	requireModulesEqual(t, want.Values.RootModule, got.Values.RootModule)
}

// requireModulesEqual compares the fields Coder consumes, ignoring sensitive
// value markers which readStateFile does not populate.
func requireModulesEqual(t *testing.T, want, got *tfjson.StateModule) {
	t.Helper()
	require.Equal(t, want.Address, got.Address)
	require.Len(t, got.Resources, len(want.Resources), "resources in %q", want.Address)
	for i := range want.Resources {
		w, g := want.Resources[i], got.Resources[i]
		require.Equal(t, w.Address, g.Address)
		require.Equal(t, w.Mode, g.Mode)
		require.Equal(t, w.Type, g.Type)
		require.Equal(t, w.Name, g.Name)
		require.Equal(t, w.Index, g.Index)
		require.Equal(t, w.ProviderName, g.ProviderName)
		require.Equal(t, w.SchemaVersion, g.SchemaVersion)
		require.Equal(t, w.AttributeValues, g.AttributeValues, "values of %s", w.Address)
		require.Equal(t, w.DependsOn, g.DependsOn, "depends_on of %s", w.Address)
		require.Equal(t, w.Tainted, g.Tainted)
		require.Equal(t, w.DeposedKey, g.DeposedKey)
	}
	require.Len(t, got.ChildModules, len(want.ChildModules), "child modules of %q", want.Address)
	for i := range want.ChildModules {
		requireModulesEqual(t, want.ChildModules[i], got.ChildModules[i])
	}
}
