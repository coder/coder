//go:build linux || darwin

package tfstate_test

import (
	"context"
	"encoding/json"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hashicorp/go-version"
	tfjson "github.com/hashicorp/terraform-json"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/provisioner/terraform"
	"github.com/coder/coder/v2/provisioner/terraform/tfstate"
	"github.com/coder/coder/v2/provisionersdk/proto"
	"github.com/coder/coder/v2/provisionersdk/tfpath"
	"github.com/coder/coder/v2/testutil"
)

// templateDir holds the template that TestRead applies.
const templateDir = "testdata/ordering"

// readTemplateDir returns the files of the template in dir, keyed by their
// slash-separated path.
func readTemplateDir(t *testing.T, dir string) map[string]string {
	t.Helper()
	files := map[string]string{}
	fsys := os.DirFS(dir)
	require.NoError(t, fs.WalkDir(fsys, ".", func(path string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		content, err := fs.ReadFile(fsys, path)
		files[path] = string(content)
		return err
	}))
	return files
}

// installedTerraform returns the path of the installed Terraform, if Read
// supports the state it writes.
func installedTerraform(ctx context.Context) (string, error) {
	path, err := exec.LookPath("terraform")
	if err != nil {
		return "", err
	}
	out, err := exec.CommandContext(ctx, path, "version", "-json").Output()
	if err != nil {
		return "", err
	}
	var installed tfjson.VersionOutput
	if err := json.Unmarshal(out, &installed); err != nil {
		return "", err
	}
	v, err := version.NewVersion(installed.Version)
	if err != nil {
		return "", err
	}
	if !tfstate.SupportsTerraformVersion(v, terraform.MaxTerraformVersion) {
		return "", xerrors.Errorf("terraform %s is not supported", v)
	}
	return path, nil
}

// workDir runs Terraform in a template's directory.
type workDir struct {
	files      tfpath.Layout
	binaryPath string
	env        []string
}

// run returns what a Terraform command writes to standard output.
func (w workDir) run(ctx context.Context, args ...string) ([]byte, error) {
	// #nosec G204 - The test chose this Terraform binary.
	cmd := exec.CommandContext(ctx, w.binaryPath, args...)
	cmd.Dir = w.files.WorkDirectory()
	cmd.Env = w.env
	var stderr strings.Builder
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return nil, xerrors.Errorf("terraform %s: %w: %s", args[0], err, stderr.String())
	}
	return out, nil
}

// show returns the root module that `terraform show -json` reports, or no
// module for an empty state.
func (w workDir) show(ctx context.Context, t *testing.T) []*tfjson.StateModule {
	t.Helper()
	out, err := w.run(ctx, "show", "-json", "-no-color")
	require.NoError(t, err)
	state := &tfjson.State{}
	require.NoError(t, json.Unmarshal(out, state))
	if state.Values == nil {
		return nil
	}
	return []*tfjson.StateModule{state.Values.RootModule}
}

// resources returns the resources of modules and of the modules below them.
func resources(modules []*tfjson.StateModule) []*tfjson.StateResource {
	var all []*tfjson.StateResource
	for _, module := range modules {
		all = append(all, module.Resources...)
		all = append(all, resources(module.ChildModules)...)
	}
	return all
}

func TestRead(t *testing.T) {
	t.Parallel()

	t.Run("MissingFile", func(t *testing.T) {
		t.Parallel()
		modules, err := tfstate.Read(filepath.Join(t.TempDir(), "terraform.tfstate"), terraform.MaxTerraformVersion)
		require.NoError(t, err)
		require.Empty(t, modules)
	})

	t.Run("NoResources", func(t *testing.T) {
		t.Parallel()
		path := filepath.Join(t.TempDir(), "terraform.tfstate")
		require.NoError(t, os.WriteFile(path, []byte(`{"version":4,"terraform_version":"1.16.2","resources":[]}`), 0o600))
		modules, err := tfstate.Read(path, terraform.MaxTerraformVersion)
		require.NoError(t, err)
		require.Empty(t, modules)
	})

	// State files that Read leaves to `terraform show`.
	withResource := func(resource string) string {
		return `{"version":4,"terraform_version":"1.16.2","resources":[` + resource + `]}`
	}
	rejected := []struct {
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
	for _, test := range rejected {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			path := filepath.Join(t.TempDir(), "terraform.tfstate")
			require.NoError(t, os.WriteFile(path, []byte(test.content), 0o600))
			_, err := tfstate.Read(path, terraform.MaxTerraformVersion)
			require.ErrorContains(t, err, test.wantErr)
		})
	}

	t.Run("Unreadable", func(t *testing.T) {
		t.Parallel()
		_, err := tfstate.Read(t.TempDir(), terraform.MaxTerraformVersion)
		require.ErrorContains(t, err, "read state file")
	})

	// MatchesTerraformShow applies the template in templateDir with real
	// Terraform and checks that Read returns what `terraform show -json`
	// shows, and that both convert to the same resources.
	t.Run("MatchesTerraformShow", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitSuperLong)
		logger := slogtest.Make(t, nil)

		binaryPath, err := installedTerraform(ctx)
		if err != nil {
			// Read leaves other versions to `terraform show`.
			binaryPath, err = terraform.Install(ctx, logger, false, t.TempDir(), terraform.TerraformVersion, "")
			require.NoError(t, err)
		}

		files := readTemplateDir(t, templateDir)
		dir := t.TempDir()
		for name, content := range files {
			path := filepath.Join(dir, filepath.FromSlash(name))
			require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
			require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
		}

		// Install providers from a local mirror, and keep the CODER_* variables
		// of a surrounding workspace away from the coder provider.
		cacheRootDir := filepath.Join(testutil.PersistentCacheDir(t), "terraform_statefile_test")
		env := []string{"TF_CLI_CONFIG_FILE=" + testutil.CacheTFProviders(t, cacheRootDir, t.Name(), files)}
		for _, variable := range os.Environ() {
			if !strings.HasPrefix(variable, "CODER_") {
				env = append(env, variable)
			}
		}
		w := workDir{files: tfpath.Layout(dir), binaryPath: binaryPath, env: env}

		_, err = w.run(ctx, "init", "-no-color", "-input=false")
		require.NoError(t, err)
		_, err = w.run(ctx, "apply", "-no-color", "-input=false", "-auto-approve")
		require.NoError(t, err)
		_, converted := requireReadMatchesShow(ctx, t, w)
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
		require.Len(t, agent.ExtraEnvs, 11+18)   // coder_env.counted and coder_env.keys
		require.Len(t, agent.Scripts, 21+11+2+1) // module.child, module.counted, the two wrap modules, and module.deep

		// A failed replacement leaves a tainted and a deposed object.
		_, err = w.run(ctx, "apply", "-no-color", "-input=false", "-auto-approve",
			"-replace=coder_agent.replaced", "-replace=terraform_data.replaced[0]", "-var=fail_provisioner=true")
		require.Error(t, err)
		modules, _ := requireReadMatchesShow(ctx, t, w)
		replaced := map[string][]*tfjson.StateResource{}
		for _, resource := range resources(modules) {
			if resource.Name == "replaced" {
				replaced[resource.Type] = append(replaced[resource.Type], resource)
			}
		}
		agents := replaced["coder_agent"]
		require.Len(t, agents, 2)
		require.True(t, agents[0].Tainted)
		require.Empty(t, agents[0].DeposedKey)
		require.False(t, agents[1].Tainted)
		require.NotEmpty(t, agents[1].DeposedKey)
		// Both objects of the first instance come before the second instance.
		counted := replaced["terraform_data"]
		require.Len(t, counted, 3)
		require.True(t, counted[0].Tainted)
		require.NotEmpty(t, counted[1].DeposedKey)
		require.Equal(t, "terraform_data.replaced[1]", counted[2].Address)

		// A delete build leaves an empty state.
		_, err = w.run(ctx, "destroy", "-no-color", "-input=false", "-auto-approve")
		require.NoError(t, err)
		require.Empty(t, w.show(ctx, t))
		read, err := tfstate.Read(w.files.StateFilePath(), terraform.MaxTerraformVersion)
		require.NoError(t, err)
		require.Empty(t, read)
	})
}

// requireReadMatchesShow checks that the state file in w reads the same as
// `terraform show -json` shows it, apart from the fields Read leaves out, and
// that both convert to the same resources. It returns what Read returned,
// with dynamically typed attributes unwrapped, and its conversion.
func requireReadMatchesShow(ctx context.Context, t *testing.T, w workDir) ([]*tfjson.StateModule, *terraform.State) {
	t.Helper()

	shown := w.show(ctx, t)
	read, err := tfstate.Read(w.files.StateFilePath(), terraform.MaxTerraformVersion)
	require.NoError(t, err)
	require.NotEmpty(t, resources(read))

	rawGraph, err := w.run(ctx, "graph", "-type=plan")
	require.NoError(t, err)
	logger := slogtest.Make(t, nil)
	wantConverted, err := terraform.ConvertState(ctx, shown, string(rawGraph), logger)
	require.NoError(t, err)
	gotConverted, err := terraform.ConvertState(ctx, read, string(rawGraph), logger)
	require.NoError(t, err)
	wantJSON, err := json.Marshal(wantConverted)
	require.NoError(t, err)
	gotJSON, err := json.Marshal(gotConverted)
	require.NoError(t, err)
	require.JSONEq(t, string(wantJSON), string(gotJSON))

	// Drop what Read leaves out because Coder doesn't read it.
	for _, r := range resources(shown) {
		r.ProviderName = ""
		r.SchemaVersion = 0
		r.SensitiveValues = nil
		r.IdentitySchemaVersion = nil
		r.IdentityValues = nil
	}
	for _, r := range resources(read) {
		if r.Type != "terraform_data" {
			continue
		}
		// Read returns dynamically typed attributes wrapped with their type,
		// as documented.
		for _, name := range []string{"input", "output"} {
			wrapped, ok := r.AttributeValues[name].(map[string]any)
			if !ok {
				continue
			}
			require.Contains(t, wrapped, "type", "%s.%s", r.Address, name)
			r.AttributeValues[name] = wrapped["value"]
		}
	}
	require.Equal(t, shown, read)
	return read, gotConverted
}
