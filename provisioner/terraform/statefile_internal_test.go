package terraform

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	tfjson "github.com/hashicorp/terraform-json"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace/noop"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/provisionersdk/proto"
	"github.com/coder/coder/v2/provisionersdk/tfpath"
	"github.com/coder/coder/v2/testutil"
)

func Test_readStateFile_EmptyState(t *testing.T) {
	t.Parallel()

	// `terraform show -json` reports an empty state as only a format version.
	empty := &tfjson.State{FormatVersion: "1.0"}

	t.Run("NoResources", func(t *testing.T) {
		t.Parallel()
		state, err := parseStateFile([]byte(`{"version":4,"terraform_version":"1.16.2","resources":[]}`))
		require.NoError(t, err)
		require.Equal(t, empty, state)
	})

	t.Run("MissingFile", func(t *testing.T) {
		t.Parallel()
		state, err := readStateFile(filepath.Join(t.TempDir(), "terraform.tfstate"))
		require.NoError(t, err)
		require.Equal(t, empty, state)
	})
}

// Test_readStateFile_Rejects covers state files that Graph must leave to
// `terraform show` instead.
func Test_readStateFile_Rejects(t *testing.T) {
	t.Parallel()

	withResource := func(resource string) string {
		return `{"version":4,"terraform_version":"1.16.2","resources":[` + resource + `]}`
	}
	tests := []struct {
		name    string
		content string
		wantErr string
	}{
		{"NotJSON", `{"version":4,`, "decode state file"},
		{"OldFormat", `{"version":3,"modules":[]}`, "unsupported state file version 3"},
		{"NewFormat", `{"version":5,"terraform_version":"2.0.0"}`, "unsupported state file version 5"},
		{"NoTerraformVersion", `{"version":4,"resources":[]}`, "parse terraform version"},
		{"UnknownMode", withResource(`{"mode":"ephemeral","type":"a","name":"b","instances":[]}`), "unsupported mode"},
		{"BoolIndexKey", withResource(`{"mode":"managed","type":"a","name":"b","instances":[{"index_key":true,"attributes":{}}]}`), "unsupported index key"},
		{"FlatmapAttributes", withResource(`{"mode":"managed","type":"a","name":"b","instances":[{"attributes_flat":{"id":"1"}}]}`), "legacy flatmap"},
		{"InvalidModule", withResource(`{"module":"module.a.b","mode":"managed","type":"a","name":"b","instances":[{"attributes":{}}]}`), "parse module address"},
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

// stateFileTestFiles is a template whose state exercises how `terraform show
// -json` orders and addresses entries: data sources, counted and keyed
// resources and modules, nested modules, and unusual keys.
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
    "tab\there", "new\nline", "cr\rlf\n", "bell\u0007", "del\u007f",
    "nbsp\u00a0", "ünïcödé", "🚀",
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

# module.wrap sorts before module.wrap-sibling, but holds no resources itself,
# and the state file lists module.wrap.module.grandchild after
# module.wrap-sibling.
module "wrap" {
  source   = "./child"
  agent_id = coder_agent.main.id
}

module "wrap-sibling" {
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
	if err != nil || !stateFileTerraformVersionSupported(binary.version) {
		// readStateFile leaves other versions to `terraform show`.
		path, err := Install(ctx, logger, false, t.TempDir(), TerraformVersion, "")
		require.NoError(t, err)
		binary = &systemBinaryDetails{absolutePath: path}
	}

	dir := t.TempDir()
	for name, content := range stateFileTestFiles {
		path := filepath.Join(dir, filepath.FromSlash(name))
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
	}

	// Install providers from a local mirror, and run show and graph the way
	// Graph does.
	cacheRootDir := filepath.Join(testutil.PersistentCacheDir(t), "terraform_statefile_test")
	srv := &server{
		binaryPath:    binary.absolutePath,
		cliConfigPath: testutil.CacheTFProviders(t, cacheRootDir, t.Name(), stateFileTestFiles),
		logger:        logger,
		tracer:        noop.NewTracerProvider().Tracer(""),
	}
	e, unlock := srv.lockExecutor(tfpath.Layout(dir), database.ProvisionerJobTimingStageGraph)
	defer unlock()
	terraform := func(args ...string) error {
		cmd := e.command(ctx, args...)
		// basicEnv keeps the CODER_* variables of a surrounding workspace
		// away from the coder provider.
		cmd.Env = e.basicEnv()
		out, err := cmd.CombinedOutput()
		if err != nil {
			return xerrors.Errorf("terraform %s: %w: %s", args[0], err, out)
		}
		return nil
	}

	require.NoError(t, terraform("init", "-no-color", "-input=false"))
	require.NoError(t, terraform("apply", "-no-color", "-input=false", "-auto-approve"))
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
	require.Len(t, agent.ExtraEnvs, 11+16) // coder_env.counted and coder_env.keys
	require.Len(t, agent.Scripts, 1+11+2)  // module.child, module.counted, and the two wrap modules

	// A failed replacement leaves a tainted and a deposed object.
	require.Error(t, terraform("apply", "-no-color", "-input=false", "-auto-approve",
		"-replace=coder_agent.replaced", "-var=fail_provisioner=true"))
	state, _ := requireStateFileMatchesShow(ctx, t, e)
	var replaced []*tfjson.StateResource
	for _, resource := range state.Values.RootModule.Resources {
		if resource.Address == "coder_agent.replaced" {
			replaced = append(replaced, resource)
		}
	}
	require.Len(t, replaced, 2)
	require.True(t, replaced[0].Tainted)
	require.Empty(t, replaced[0].DeposedKey)
	require.False(t, replaced[1].Tainted)
	require.NotEmpty(t, replaced[1].DeposedKey)

	// A delete build leaves an empty state.
	require.NoError(t, terraform("destroy", "-no-color", "-input=false", "-auto-approve"))
	shown, err := e.state(ctx, ctx)
	require.NoError(t, err)
	require.Nil(t, shown.Values)
	read, err := readStateFile(e.files.StateFilePath())
	require.NoError(t, err)
	require.Equal(t, shown, read)
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
	require.NoError(t, forEachResource(shown.Values.RootModule, func(r *tfjson.StateResource) error {
		r.ProviderName = ""
		r.SchemaVersion = 0
		r.SensitiveValues = nil
		r.IdentitySchemaVersion = nil
		r.IdentityValues = nil
		return nil
	}))
	require.NoError(t, forEachResource(read.Values.RootModule, func(r *tfjson.StateResource) error {
		if r.Type != "terraform_data" {
			return nil
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
		return nil
	}))
	require.Equal(t, shown, read)
	return read, gotConverted
}
