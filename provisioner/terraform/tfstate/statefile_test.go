//go:build linux || darwin

package tfstate_test

import (
	"context"
	"encoding/json"
	"fmt"
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

// templateDir holds the template that TestReadMatchesTerraformShow applies.
const templateDir = "testdata/ordering"

// fixturesDir holds the templates that ConvertState's golden tests cover.
const fixturesDir = "../testdata/resources"

// fixtureProviders requires the providers that the templates in fixturesDir
// require, so that one provider mirror serves all of them. The coder
// provider's version is the one their golden files are generated with.
const fixtureProviders = `terraform {
  required_providers {
    coder = {
      source  = "coder/coder"
      version = "%s"
    }
    null = {
      source = "hashicorp/null"
    }
    docker = {
      source  = "kreuzwerker/docker"
      version = "~> 2.22"
    }
  }
}
`

// readTemplateDir returns the .tf files of the template in dir, keyed by
// their slash-separated path. It leaves out what a local Terraform run may
// have left behind, such as .terraform.
func readTemplateDir(t *testing.T, dir string) map[string]string {
	t.Helper()
	files := map[string]string{}
	fsys := os.DirFS(dir)
	require.NoError(t, fs.WalkDir(fsys, ".", func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			if path != "." && strings.HasPrefix(entry.Name(), ".") {
				return fs.SkipDir
			}
			return nil
		}
		if filepath.Ext(path) != ".tf" {
			return nil
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

// terraformBinary returns the path of a Terraform whose state Read supports:
// the installed one, or else the version the provisioner installs.
func terraformBinary(ctx context.Context, t *testing.T) string {
	t.Helper()
	binaryPath, err := installedTerraform(ctx)
	if err != nil {
		// Read leaves other versions to `terraform show`.
		binaryPath, err = terraform.Install(ctx, slogtest.Make(t, nil), false, t.TempDir(), terraform.TerraformVersion, "")
		require.NoError(t, err)
	}
	return binaryPath
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

// applyTemplate applies the template made of files in a directory of its own.
// Terraform installs providers from the local mirror that the CLI
// configuration at cliConfigPath names.
func applyTemplate(ctx context.Context, t *testing.T, binaryPath, cliConfigPath string, files map[string]string) workDir {
	t.Helper()
	dir := t.TempDir()
	for name, content := range files {
		path := filepath.Join(dir, filepath.FromSlash(name))
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
	}

	// Keep the CODER_* variables of a surrounding workspace away from the
	// coder provider.
	env := []string{"TF_CLI_CONFIG_FILE=" + cliConfigPath}
	for _, variable := range os.Environ() {
		if !strings.HasPrefix(variable, "CODER_") {
			env = append(env, variable)
		}
	}
	w := workDir{files: tfpath.Layout(dir), binaryPath: binaryPath, env: env}

	_, err := w.run(ctx, "init", "-no-color", "-input=false")
	require.NoError(t, err)
	_, err = w.run(ctx, "apply", "-no-color", "-input=false", "-auto-approve")
	require.NoError(t, err)
	return w
}

// TestReadMatchesTerraformShow applies the template in templateDir with real
// Terraform and checks that Read returns what `terraform show -json` shows,
// and that both convert to the same resources.
func TestReadMatchesTerraformShow(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitSuperLong)

	files := readTemplateDir(t, templateDir)
	cacheRootDir := filepath.Join(testutil.PersistentCacheDir(t), "terraform_statefile_test")
	cliConfigPath := testutil.CacheTFProviders(t, cacheRootDir, t.Name(), files)
	w := applyTemplate(ctx, t, terraformBinary(ctx, t), cliConfigPath, files)
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
	require.Len(t, agent.ExtraEnvs, 11+16) // coder_env.counted and coder_env.keys
	require.Len(t, agent.Scripts, 19+11+2) // module.child, module.counted, and the two wrap modules

	// A failed replacement leaves a tainted and a deposed object.
	_, err := w.run(ctx, "apply", "-no-color", "-input=false", "-auto-approve",
		"-replace=coder_agent.replaced", "-var=fail_provisioner=true")
	require.Error(t, err)
	modules, _ := requireReadMatchesShow(ctx, t, w)
	var replaced []*tfjson.StateResource
	for _, resource := range resources(modules) {
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
	_, err = w.run(ctx, "destroy", "-no-color", "-input=false", "-auto-approve")
	require.NoError(t, err)
	require.Empty(t, w.show(ctx, t))
	read, err := tfstate.Read(w.files.StateFilePath(), terraform.MaxTerraformVersion)
	require.NoError(t, err)
	require.Empty(t, read)
}

// TestReadMatchesTerraformShowOnFixtures makes the same comparison for the
// templates the provisioner already has fixtures for.
func TestReadMatchesTerraformShowOnFixtures(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitSuperLong)
	binaryPath := terraformBinary(ctx, t)
	cacheRootDir := filepath.Join(testutil.PersistentCacheDir(t), "terraform_statefile_test")

	providerVersion, err := os.ReadFile("../testdata/provider-version.txt")
	require.NoError(t, err)
	fixturesCLIConfigPath := testutil.CacheTFProviders(t, cacheRootDir, t.Name(), map[string]string{
		"main.tf": fmt.Sprintf(fixtureProviders, strings.TrimSpace(string(providerVersion))),
	})
	entries, err := os.ReadDir(fixturesDir)
	require.NoError(t, err)
	for _, entry := range entries {
		// Applying kubernetes-metadata takes a cluster.
		if !entry.IsDir() || entry.Name() == "kubernetes-metadata" {
			continue
		}
		t.Run(entry.Name(), func(t *testing.T) {
			t.Parallel()
			ctx := testutil.Context(t, testutil.WaitSuperLong)
			files := readTemplateDir(t, filepath.Join(fixturesDir, entry.Name()))
			requireReadMatchesShow(ctx, t, applyTemplate(ctx, t, binaryPath, fixturesCLIConfigPath, files))
		})
	}

	// The script ordering template requires a newer coder provider than
	// the others.
	t.Run("script-order-modules", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitSuperLong)
		files := readTemplateDir(t, "../testdata/script-order-modules")
		cliConfigPath := testutil.CacheTFProviders(t, cacheRootDir, t.Name(), files)
		requireReadMatchesShow(ctx, t, applyTemplate(ctx, t, binaryPath, cliConfigPath, files))
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
	wantConverted, wantErr := terraform.ConvertState(ctx, shown, string(rawGraph), logger)
	gotConverted, gotErr := terraform.ConvertState(ctx, read, string(rawGraph), logger)
	if wantErr != nil {
		// Some fixtures hold state that ConvertState rejects.
		require.EqualError(t, gotErr, wantErr.Error())
	} else {
		require.NoError(t, gotErr)
	}
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
