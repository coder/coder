package cli_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/cli"
	"github.com/coder/coder/v2/cli/clitest"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/provisioner/echo"
	"github.com/coder/coder/v2/provisionersdk/proto"
	"github.com/coder/coder/v2/pty/ptytest"
	"github.com/coder/coder/v2/testutil"
)

func TestTemplatePresets(t *testing.T) {
	t.Parallel()

	t.Run("NoPresets", func(t *testing.T) {
		t.Parallel()

		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
		owner := coderdtest.CreateFirstUser(t, client)
		member, _ := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID)

		// Given: a template version without presets
		version := coderdtest.CreateTemplateVersion(t, client, owner.OrganizationID, templateWithPresets([]*proto.Preset{}))
		_ = coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
		template := coderdtest.CreateTemplate(t, client, owner.OrganizationID, version.ID)

		// When: listing presets for that template
		inv, root := clitest.New(t, "templates", "presets", "list", template.Name)
		clitest.SetupConfig(t, member, root)

		pty := ptytest.New(t).Attach(inv)
		doneChan := make(chan struct{})
		var runErr error
		go func() {
			defer close(doneChan)
			runErr = inv.Run()
		}()
		<-doneChan
		require.NoError(t, runErr)

		// Should return a message when no presets are found for the given template and version.
		notFoundMessage := fmt.Sprintf("No presets found for template %q and template-version %q.", template.Name, version.Name)
		pty.ExpectRegexMatch(notFoundMessage)
	})

	t.Run("ListsPresetsForDefaultTemplateVersion", func(t *testing.T) {
		t.Parallel()

		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
		owner := coderdtest.CreateFirstUser(t, client)
		member, _ := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID)

		// Given: an active template version that includes presets
		presets := []*proto.Preset{
			{
				Name: "preset-multiple-params",
				Parameters: []*proto.PresetParameter{
					{
						Name:  "k1",
						Value: "v1",
					}, {
						Name:  "k2",
						Value: "v2",
					},
				},
			},
			{
				Name:    "preset-default",
				Default: true,
				Parameters: []*proto.PresetParameter{
					{
						Name:  "k1",
						Value: "v2",
					},
				},
				Prebuild: &proto.Prebuild{
					Instances: 0,
				},
			},
			{
				Name:        "preset-prebuilds",
				Description: "Preset without parameters and 2 prebuild instances.",
				Parameters:  []*proto.PresetParameter{},
				Prebuild: &proto.Prebuild{
					Instances: 2,
				},
			},
		}
		version := coderdtest.CreateTemplateVersion(t, client, owner.OrganizationID, templateWithPresets(presets))
		_ = coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
		template := coderdtest.CreateTemplate(t, client, owner.OrganizationID, version.ID)
		require.Equal(t, version.ID, template.ActiveVersionID)

		// When: listing presets for that template
		inv, root := clitest.New(t, "templates", "presets", "list", template.Name)
		clitest.SetupConfig(t, member, root)

		pty := ptytest.New(t).Attach(inv)
		doneChan := make(chan struct{})
		var runErr error
		go func() {
			defer close(doneChan)
			runErr = inv.Run()
		}()

		<-doneChan
		require.NoError(t, runErr)

		// Should: return the active version's presets sorted by name
		message := fmt.Sprintf("Showing presets for template %q and template version %q.", template.Name, version.Name)
		pty.ExpectMatch(message)
		pty.ExpectRegexMatch(`preset-default\s+k1=v2\s+true\s+0`)
		// The parameter order is not guaranteed in the output, so we match both possible orders
		pty.ExpectRegexMatch(`preset-multiple-params\s+(k1=v1,k2=v2)|(k2=v2,k1=v1)\s+false\s+-`)
		pty.ExpectRegexMatch(`preset-prebuilds\s+Preset without parameters and 2 prebuild instances.\s+\s+false\s+2`)
	})

	t.Run("ListsPresetsForSpecifiedTemplateVersion", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitMedium)

		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
		owner := coderdtest.CreateFirstUser(t, client)
		member, _ := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID)

		// Given: a template with an active version that has no presets,
		// and another template version that includes presets
		presets := []*proto.Preset{
			{
				Name: "preset-multiple-params",
				Parameters: []*proto.PresetParameter{
					{
						Name:  "k1",
						Value: "v1",
					}, {
						Name:  "k2",
						Value: "v2",
					},
				},
			},
			{
				Name:    "preset-default",
				Default: true,
				Parameters: []*proto.PresetParameter{
					{
						Name:  "k1",
						Value: "v2",
					},
				},
				Prebuild: &proto.Prebuild{
					Instances: 0,
				},
			},
			{
				Name:        "preset-prebuilds",
				Description: "Preset without parameters and 2 prebuild instances.",
				Parameters:  []*proto.PresetParameter{},
				Prebuild: &proto.Prebuild{
					Instances: 2,
				},
			},
		}
		// Given: first template version with presets
		version := coderdtest.CreateTemplateVersion(t, client, owner.OrganizationID, templateWithPresets(presets))
		_ = coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
		template := coderdtest.CreateTemplate(t, client, owner.OrganizationID, version.ID)
		// Given: second template version without presets
		activeVersion := coderdtest.UpdateTemplateVersion(t, client, owner.OrganizationID, templateWithPresets([]*proto.Preset{}), template.ID)
		_ = coderdtest.AwaitTemplateVersionJobCompleted(t, client, activeVersion.ID)
		// Given: second template version is the active version
		err := client.UpdateActiveTemplateVersion(ctx, template.ID, codersdk.UpdateActiveTemplateVersion{
			ID: activeVersion.ID,
		})
		require.NoError(t, err)
		updatedTemplate, err := client.Template(ctx, template.ID)
		require.NoError(t, err)
		require.Equal(t, activeVersion.ID, updatedTemplate.ActiveVersionID)
		// Given: template has two versions
		templateVersions, err := client.TemplateVersionsByTemplate(ctx, codersdk.TemplateVersionsByTemplateRequest{
			TemplateID: updatedTemplate.ID,
		})
		require.NoError(t, err)
		require.Len(t, templateVersions, 2)

		// When: listing presets for a specific template and its specified version
		inv, root := clitest.New(t, "templates", "presets", "list", updatedTemplate.Name, "--template-version", version.Name)
		clitest.SetupConfig(t, member, root)

		pty := ptytest.New(t).Attach(inv)
		doneChan := make(chan struct{})
		var runErr error
		go func() {
			defer close(doneChan)
			runErr = inv.Run()
		}()

		<-doneChan
		require.NoError(t, runErr)

		// Should: return the specified version's presets sorted by name
		message := fmt.Sprintf("Showing presets for template %q and template version %q.", template.Name, version.Name)
		pty.ExpectMatch(message)
		pty.ExpectRegexMatch(`preset-default\s+k1=v2\s+true\s+0`)
		// The parameter order is not guaranteed in the output, so we match both possible orders
		pty.ExpectRegexMatch(`preset-multiple-params\s+(k1=v1,k2=v2)|(k2=v2,k1=v1)\s+false\s+-`)
		pty.ExpectRegexMatch(`preset-prebuilds\s+Preset without parameters and 2 prebuild instances.\s+\s+false\s+2`)
	})

	t.Run("ListsPresetsJSON", func(t *testing.T) {
		t.Parallel()

		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
		owner := coderdtest.CreateFirstUser(t, client)
		member, _ := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID)

		// Given: an active template version that includes presets
		preset := proto.Preset{
			Name:        "preset-default",
			Description: "Preset with parameters and 2 prebuild instances.",
			Icon:        "/emojis/1f60e.png",
			Default:     true,
			Parameters: []*proto.PresetParameter{
				{
					Name:  "k1",
					Value: "v2",
				},
			},
			Prebuild: &proto.Prebuild{
				Instances: 2,
			},
		}

		version := coderdtest.CreateTemplateVersion(t, client, owner.OrganizationID, templateWithPresets([]*proto.Preset{&preset}))
		_ = coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
		template := coderdtest.CreateTemplate(t, client, owner.OrganizationID, version.ID)
		require.Equal(t, version.ID, template.ActiveVersionID)

		// When: listing presets for that template
		inv, root := clitest.New(t, "templates", "presets", "list", template.Name, "-o", "json")
		clitest.SetupConfig(t, member, root)

		buf := bytes.NewBuffer(nil)
		inv.Stdout = buf
		doneChan := make(chan struct{})
		var runErr error
		go func() {
			defer close(doneChan)
			runErr = inv.Run()
		}()

		<-doneChan
		require.NoError(t, runErr)

		// Should: return the active version's preset
		var jsonPresets []cli.TemplatePresetRow
		err := json.Unmarshal(buf.Bytes(), &jsonPresets)
		require.NoError(t, err, "unmarshal JSON output")
		require.Len(t, jsonPresets, 1)

		jsonPreset := jsonPresets[0].TemplatePreset
		require.Equal(t, preset.Name, jsonPreset.Name)
		require.Equal(t, preset.Description, jsonPreset.Description)
		require.Equal(t, preset.Icon, jsonPreset.Icon)
		require.Equal(t, preset.Default, jsonPreset.Default)
		require.Equal(t, len(preset.Parameters), len(jsonPreset.Parameters))
		require.Equal(t, preset.Parameters[0].Name, jsonPreset.Parameters[0].Name)
		require.Equal(t, preset.Parameters[0].Value, jsonPreset.Parameters[0].Value)
		require.Equal(t, int(preset.Prebuild.Instances), *jsonPreset.DesiredPrebuildInstances)
	})
}

func templateWithPresets(presets []*proto.Preset) *echo.Responses {
	return &echo.Responses{
		Parse: echo.ParseComplete,
		ProvisionPlan: []*proto.Response{
			{
				Type: &proto.Response_Graph{
					Graph: &proto.GraphComplete{
						Presets: presets,
					},
				},
			},
		},
	}
}
