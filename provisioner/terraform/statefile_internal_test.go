package terraform

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	tfjson "github.com/hashicorp/terraform-json"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace/noop"

	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/provisionersdk/proto"
	"github.com/coder/coder/v2/provisionersdk/tfpath"
	"github.com/coder/coder/v2/testutil"
)

// testRawState exercises every shape of state file entry Coder may see:
// root and nested module instances (including an intermediate module that
// holds no resources itself), count and for_each indexes, data sources,
// a tainted instance, a deposed object, and an aliased provider. Resources
// and instances are listed out of order, and a module's resources are not
// adjacent, so the conversion has to sort and group them.
const testRawState = `{
  "version": 4,
  "terraform_version": "1.16.2",
  "serial": 7,
  "lineage": "8f7a6a2e-1111-2222-3333-444444444444",
  "outputs": {},
  "resources": [
    {
      "module": "module.outer[\"a.module.b\"].module.inner",
      "mode": "managed",
      "type": "coder_app",
      "name": "code",
      "provider": "module.outer[\"a.module.b\"].module.inner.provider[\"registry.terraform.io/coder/coder\"]",
      "instances": [
        {
          "index_key": "web \"ui\"",
          "schema_version": 0,
          "attributes": {"id": "app-2", "slug": "web"}
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
          "index_key": 10,
          "schema_version": 2,
          "attributes": {"id": "c-10"}
        },
        {
          "index_key": 0,
          "deposed": "abcdef12",
          "schema_version": 2,
          "attributes": {"id": "c-old"}
        },
        {
          "index_key": 2,
          "schema_version": 2,
          "attributes": {"id": "c-2"}
        },
        {
          "index_key": 0,
          "schema_version": 2,
          "status": "tainted",
          "attributes": {"id": "c-0", "ports": [{"internal": 80, "external": 8080}]},
          "dependencies": ["coder_agent.main", "data.coder_workspace.me"]
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
        }
      ]
    },
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
	// name, then integer keys numerically, with the deposed object after
	// its current one.
	require.Equal(t, []string{
		"data.coder_workspace.me",
		"coder_agent.main",
		"docker_container.workspace[0]",
		"docker_container.workspace[0]",
		"docker_container.workspace[2]",
		"docker_container.workspace[10]",
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
	require.Equal(t, map[string]any{
		"id":             "agent-1",
		"arch":           "amd64",
		"os":             "linux",
		"token":          "secret",
		"env":            map[string]any{"A": "1"},
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
	require.False(t, deposed.Tainted)
	require.Equal(t, "c-old", deposed.AttributeValues["id"])
	require.Equal(t, float64(10), root.Resources[5].Index)

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

func Test_readStateFile_EmptyState(t *testing.T) {
	t.Parallel()

	t.Run("NoResources", func(t *testing.T) {
		t.Parallel()
		// 1.4.0 is the oldest Terraform whose state file is converted.
		state, err := convertRawState([]byte(`{"version":4,"terraform_version":"1.4.0","resources":[]}`))
		require.NoError(t, err)
		require.Equal(t, "1.4.0", state.TerraformVersion)
		require.Nil(t, state.Values, "terraform show omits values for an empty state")
	})

	t.Run("MissingFile", func(t *testing.T) {
		t.Parallel()
		state, err := readStateFile(filepath.Join(t.TempDir(), "terraform.tfstate"))
		require.NoError(t, err)
		require.Nil(t, state.Values)
	})
}

// Test_readStateFile_Rejects covers state files that Graph must leave to
// `terraform show` instead.
func Test_readStateFile_Rejects(t *testing.T) {
	t.Parallel()

	withResource := func(resource string) string {
		return `{"version":4,"terraform_version":"1.16.2","resources":[` + resource + `]}`
	}
	const provider = `"provider":"provider[\"registry.terraform.io/coder/coder\"]"`
	tests := []struct {
		name    string
		content string
		wantErr string
	}{
		{"NotJSON", `{"version":4,`, "decode state file"},
		{"OldFormat", `{"version":3,"modules":[]}`, "unsupported state file version 3"},
		{"NewFormat", `{"version":5,"terraform_version":"2.0.0"}`, "unsupported state file version 5"},
		{"NoTerraformVersion", `{"version":4,"resources":[]}`, "parse terraform version"},
		{"OldTerraform", `{"version":4,"terraform_version":"1.3.10","resources":[]}`, "written by terraform 1.3.10"},
		{"UnknownMode", withResource(`{"mode":"ephemeral","type":"a","name":"b",` + provider + `,"instances":[]}`), "unsupported resource mode"},
		{"LegacyProvider", withResource(`{"mode":"managed","type":"a","name":"b","provider":"provider.coder","instances":[]}`), "unsupported provider reference"},
		{"UnterminatedProvider", withResource(`{"mode":"managed","type":"a","name":"b","provider":"provider[\"x/y/z","instances":[]}`), "unsupported provider reference"},
		{"BoolIndexKey", withResource(`{"mode":"managed","type":"a","name":"b",` + provider + `,"instances":[{"index_key":true,"attributes":{}}]}`), "unsupported index key"},
		{"FlatmapAttributes", withResource(`{"mode":"managed","type":"a","name":"b",` + provider + `,"instances":[{"attributes_flat":{"id":"1"}}]}`), "legacy flatmap"},
		{"InvalidModule", withResource(`{"module":"module.a.b","mode":"managed","type":"a","name":"b",` + provider + `,"instances":[{"attributes":{}}]}`), "parse module address"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			path := filepath.Join(t.TempDir(), "terraform.tfstate")
			require.NoError(t, os.WriteFile(path, []byte(test.content), 0o600))
			_, err := readStateFile(path)
			require.ErrorContains(t, err, test.wantErr)
		})
	}

	t.Run("Unreadable", func(t *testing.T) {
		t.Parallel()
		_, err := readStateFile(t.TempDir())
		require.ErrorContains(t, err, "read state file")
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
		got, err := parentModule(input)
		require.NoError(t, err, input)
		require.Equal(t, want, got, input)
	}

	_, err := parentModule("module.a.coder_agent.main")
	require.Error(t, err, "only module paths are accepted")
}

func Test_hclQuotedString(t *testing.T) {
	t.Parallel()
	// Expected values are the instance keys `terraform show -json` reports.
	for input, want := range map[string]string{
		"":              `""`,
		"code":          `"code"`,
		"web ui":        `"web ui"`,
		`web "ui"`:      `"web \"ui\""`,
		`back\slash`:    `"back\\slash"`,
		"a${b}":         `"a$${b}"`,
		"c%{d}":         `"c%%{d}"`,
		"$x and %y":     `"$x and %y"`,
		"tab\there":     `"tab\there"`,
		"bell\x07":      `"bell\u0007"`,
		"tag\U000E0001": `"tag\U000e0001"`,
	} {
		require.Equal(t, want, hclQuotedString(input), input)
	}
}

// stateFileTestFiles is a template that leaves every kind of entry Coder
// reads from state in the state file, with keys and modules that exercise
// how `terraform show -json` orders and addresses them.
var stateFileTestFiles = map[string]string{
	"main.tf": `
terraform {
  required_providers {
    coder = {
      source  = "coder/coder"
      version = "2.15.0"
    }
  }
}

variable "fail_provisioner" {
  type    = bool
  default = false
}

data "coder_workspace" "me" {}

data "coder_parameter" "region" {
  name    = "region"
  default = "us"
  option {
    name  = "US"
    value = "us"
  }
  option {
    name  = "EU"
    value = "eu"
  }
}

data "coder_parameter" "cpus" {
  name    = "cpus"
  type    = "number"
  default = 2
  validation {
    min = 1
    max = 8
  }
}

data "coder_external_auth" "github" {
  id       = "github"
  optional = true
}

resource "coder_agent" "main" {
  arch = "amd64"
  os   = "linux"
  env  = { REGION = data.coder_parameter.region.value }
  metadata {
    key          = "load"
    display_name = "Load"
    script       = "uptime"
    interval     = 10
  }
}

# terraform_data.input is dynamically typed, so the state file records its
# type next to its value.
resource "terraform_data" "host" {
  input = coder_agent.main.token
}

resource "coder_metadata" "host" {
  resource_id = terraform_data.host.id
  item {
    key   = "region"
    value = data.coder_parameter.region.value
  }
  item {
    key       = "password"
    value     = "hunter2"
    sensitive = true
  }
}

resource "coder_app" "apps" {
  for_each     = toset(["code", "web ui"])
  agent_id     = coder_agent.main.id
  slug         = replace(each.key, " ", "-")
  display_name = each.key
  url          = "http://localhost:8080"
  healthcheck {
    url       = "http://localhost:8080/healthz"
    interval  = 5
    threshold = 3
  }
}

# Integer keys sort numerically: [10] follows [9].
resource "coder_env" "counted" {
  count    = 11
  agent_id = coder_agent.main.id
  name     = "COUNTED_${count.index}"
  value    = tostring(count.index)
}

# String keys sort bytewise and are quoted in addresses the way Terraform
# quotes them, which escapes template introducers and non-printable runes.
resource "coder_env" "keys" {
  for_each = toset([
    "", "B", "a", "web ui", "quote\"d", "back\\slash", "a$${b}", "c%%{d}",
    "tab\there", "new\nline", "bell\u0007", "del\u007f", "nbsp\u00a0",
    "ünïcödé", "🚀",
  ])
  agent_id = coder_agent.main.id
  name     = "KEY_${md5(each.key)}"
  value    = each.key
}

# When fail_provisioner is set, replacing this agent leaves a tainted new
# object and keeps the old one as a deposed object. Nothing hosts the agent:
# its current and deposed graph nodes share a label, and ConvertState picks
# either one.
resource "coder_agent" "replaced" {
  arch = "amd64"
  os   = "linux"
  lifecycle {
    create_before_destroy = true
  }
  provisioner "local-exec" {
    command = var.fail_provisioner ? "exit 1" : "exit 0"
  }
}

# module.child holds no resources itself, and its key contains ".module.".
module "child" {
  for_each = toset(["x.module.y"])
  source   = "./child"
  agent_id = coder_agent.main.id
}

# Module instances sort by address: [10] precedes [2].
module "counted" {
  count    = 11
  source   = "./child/grandchild"
  agent_id = coder_agent.main.id
}
`,
	"child/main.tf": `
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
`,
	"child/grandchild/main.tf": `
terraform {
  required_providers {
    coder = { source = "coder/coder" }
  }
}
variable "agent_id" { type = string }
data "coder_workspace" "me" {}
resource "coder_script" "deep" {
  agent_id     = var.agent_id
  display_name = "deep"
  script       = "echo deep"
  run_on_start = true
}
`,
}

// Test_readStateFile_MatchesTerraformShow applies stateFileTestFiles with
// real Terraform and checks that readStateFile returns what Graph's
// `terraform show -json` fallback returns, and that both convert to the
// same resources.
func Test_readStateFile_MatchesTerraformShow(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitSuperLong)
	logger := slogtest.Make(t, nil)

	binary, err := systemBinary(ctx)
	if err != nil {
		path, installErr := Install(ctx, logger, false, t.TempDir(), TerraformVersion, "")
		require.NoError(t, installErr, "no usable terraform binary: %v", err)
		binary = &systemBinaryDetails{absolutePath: path}
	}

	dir := t.TempDir()
	for name, content := range stateFileTestFiles {
		path := filepath.Join(dir, filepath.FromSlash(name))
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
	}

	// Install providers from a local mirror, and keep the CODER_* variables of
	// a surrounding workspace away from the coder provider.
	env := append(safeEnviron(), "TF_IN_AUTOMATION=1", "CHECKPOINT_DISABLE=1")
	cacheRootDir := filepath.Join(testutil.PersistentCacheDir(t), "terraform_statefile_test")
	cliConfigPath := testutil.CacheTFProviders(t, cacheRootDir, t.Name(), stateFileTestFiles)
	if cliConfigPath != "" {
		env = append(env, "TF_CLI_CONFIG_FILE="+cliConfigPath)
	}
	terraform := func(wantSuccess bool, args ...string) {
		t.Helper()
		//nolint:gosec // Test-controlled terraform binary and arguments.
		cmd := exec.CommandContext(ctx, binary.absolutePath, args...)
		cmd.Dir = dir
		cmd.Env = env
		out, err := cmd.CombinedOutput()
		if wantSuccess {
			require.NoError(t, err, "%s: %s", args, out)
		} else {
			require.Error(t, err, "%s: %s", args, out)
		}
	}

	// Run show and graph the way Graph does.
	srv := &server{
		binaryPath:    binary.absolutePath,
		cliConfigPath: cliConfigPath,
		logger:        logger,
		tracer:        noop.NewTracerProvider().Tracer(""),
	}
	e, unlock := srv.lockExecutor(tfpath.Layout(dir), database.ProvisionerJobTimingStageGraph)
	defer unlock()

	terraform(true, "init", "-no-color", "-input=false")
	terraform(true, "apply", "-no-color", "-input=false", "-auto-approve")
	_, converted := requireStateFileMatchesShow(ctx, t, e)
	// Make sure the conversion compared above is not trivially empty.
	require.Len(t, converted.Parameters, 2)
	require.Len(t, converted.ExternalAuthProviders, 1)
	var agent *proto.Agent
	for _, resource := range converted.Resources {
		if resource.Name == "host" && resource.Type == "terraform_data" {
			require.Len(t, resource.Agents, 1)
			agent = resource.Agents[0]
			require.Len(t, resource.Metadata, 2)
		}
	}
	require.NotNil(t, agent, "terraform_data.host has the agent")
	require.Len(t, agent.Apps, 2)
	require.Len(t, agent.ExtraEnvs, 11+15)
	require.Len(t, agent.Scripts, 12)

	terraform(false, "apply", "-no-color", "-input=false", "-auto-approve",
		"-replace=coder_agent.replaced", "-var=fail_provisioner=true")
	state, _ := requireStateFileMatchesShow(ctx, t, e)
	var replaced []*tfjson.StateResource
	for _, resource := range state.Values.RootModule.Resources {
		if resource.Address == "coder_agent.replaced" {
			replaced = append(replaced, resource)
		}
	}
	require.Len(t, replaced, 2, "the failed replacement left a new and a deposed object")
	require.True(t, replaced[0].Tainted)
	require.Empty(t, replaced[0].DeposedKey)
	require.False(t, replaced[1].Tainted)
	require.NotEmpty(t, replaced[1].DeposedKey)
}

// requireStateFileMatchesShow checks that the state file in e's working
// directory reads the same as `terraform show -json` shows it, apart from
// the fields readStateFile leaves out, and that both convert to the same
// resources. It returns the state read from the file, with dynamically
// typed attributes unwrapped, and its conversion.
func requireStateFileMatchesShow(ctx context.Context, t *testing.T, e *executor) (*tfjson.State, *State) {
	t.Helper()

	shown, err := e.state(ctx, ctx)
	require.NoError(t, err)
	read, err := readStateFile(e.files.StateFilePath())
	require.NoError(t, err)
	require.NotNil(t, read.Values)

	rawGraph, err := e.graph(ctx, ctx)
	require.NoError(t, err)
	logger := slogtest.Make(t, nil)
	wantConverted, err := ConvertState(ctx, []*tfjson.StateModule{shown.Values.RootModule}, rawGraph, logger)
	require.NoError(t, err)
	gotConverted, err := ConvertState(ctx, []*tfjson.StateModule{read.Values.RootModule}, rawGraph, logger)
	require.NoError(t, err)
	wantJSON, err := json.Marshal(wantConverted)
	require.NoError(t, err)
	gotJSON, err := json.Marshal(gotConverted)
	require.NoError(t, err)
	require.JSONEq(t, string(wantJSON), string(gotJSON))

	// Drop what readStateFile leaves out because Coder doesn't read it.
	shown.Checks = nil
	shown.Values.Outputs = nil
	forEachStateResource(shown.Values.RootModule, func(r *tfjson.StateResource) {
		r.SensitiveValues = nil
		r.IdentitySchemaVersion = nil
		r.IdentityValues = nil
	})
	// `terraform show -json` reports schema version 0 for deposed objects
	// instead of copying it from the instance. Coder doesn't read it.
	for _, state := range []*tfjson.State{shown, read} {
		forEachStateResource(state.Values.RootModule, func(r *tfjson.StateResource) {
			if r.DeposedKey != "" {
				r.SchemaVersion = 0
			}
		})
	}
	forEachStateResource(read.Values.RootModule, func(r *tfjson.StateResource) {
		if r.Type != "terraform_data" {
			return
		}
		// readStateFile returns dynamically typed attributes wrapped with
		// their type, as documented.
		for _, name := range []string{"input", "output"} {
			wrapped, ok := r.AttributeValues[name].(map[string]any)
			if !ok {
				continue
			}
			require.Contains(t, wrapped, "type", "%s.%s", r.Address, name)
			r.AttributeValues[name] = wrapped["value"]
		}
	})
	require.Equal(t, shown, read)
	return read, gotConverted
}

func forEachStateResource(m *tfjson.StateModule, fn func(*tfjson.StateResource)) {
	for _, r := range m.Resources {
		fn(r)
	}
	for _, child := range m.ChildModules {
		forEachStateResource(child, fn)
	}
}
