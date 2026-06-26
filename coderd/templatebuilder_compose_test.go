package coderd_test

import (
	"archive/tar"
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

func TestTemplateBuilderCompose(t *testing.T) {
	t.Parallel()

	t.Run("BaseOnly", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		tarData, err := client.TemplateBuilderCompose(ctx, codersdk.TemplateBuilderComposeRequest{
			BaseTemplateID: "docker",
		})
		require.NoError(t, err)
		require.NotEmpty(t, tarData)

		files := extractTarFiles(t, tarData)
		require.Contains(t, files, "main.tf")
		require.NotContains(t, files, "modules.tf")
		require.Contains(t, files["main.tf"], `resource "coder_agent"`)
	})

	t.Run("BaseWithModules", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		tarData, err := client.TemplateBuilderCompose(ctx, codersdk.TemplateBuilderComposeRequest{
			BaseTemplateID: "docker",
			Modules: []codersdk.TemplateBuilderComposeModule{
				{ID: "code-server"},
				{
					ID: "git-clone",
					Variables: map[string]string{
						"url": `"https://github.com/coder/coder"`,
					},
				},
			},
		})
		require.NoError(t, err)

		files := extractTarFiles(t, tarData)
		require.Contains(t, files, "main.tf")
		require.Contains(t, files, "modules.tf")
		require.Contains(t, files["modules.tf"], `module "code-server"`)
		require.Contains(t, files["modules.tf"], `module "git-clone"`)
		require.Contains(t, files["modules.tf"], `coder_agent.main.id`)
	})

	t.Run("UnknownBase", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		_, err := client.TemplateBuilderCompose(ctx, codersdk.TemplateBuilderComposeRequest{
			BaseTemplateID: "nonexistent",
		})
		require.Error(t, err)
		var sdkErr *codersdk.Error
		require.ErrorAs(t, err, &sdkErr)
		require.Equal(t, http.StatusBadRequest, sdkErr.StatusCode())
	})

	t.Run("UnknownModule", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		_, err := client.TemplateBuilderCompose(ctx, codersdk.TemplateBuilderComposeRequest{
			BaseTemplateID: "docker",
			Modules: []codersdk.TemplateBuilderComposeModule{
				{ID: "nonexistent-module"},
			},
		})
		require.Error(t, err)
		var sdkErr *codersdk.Error
		require.ErrorAs(t, err, &sdkErr)
		require.Equal(t, http.StatusBadRequest, sdkErr.StatusCode())
	})

	t.Run("MissingBaseTemplateID", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		_, err := client.TemplateBuilderCompose(ctx, codersdk.TemplateBuilderComposeRequest{})
		require.Error(t, err)
		var sdkErr *codersdk.Error
		require.ErrorAs(t, err, &sdkErr)
		require.Equal(t, http.StatusBadRequest, sdkErr.StatusCode())
	})

	t.Run("DisabledReturns404", func(t *testing.T) {
		t.Parallel()
		dv := coderdtest.DeploymentValues(t)
		dv.TemplateBuilder.Disabled = true

		client := coderdtest.New(t, &coderdtest.Options{
			DeploymentValues: dv,
		})
		_ = coderdtest.CreateFirstUser(t, client)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		_, err := client.TemplateBuilderCompose(ctx, codersdk.TemplateBuilderComposeRequest{
			BaseTemplateID: "docker",
		})
		require.Error(t, err)
		var sdkErr *codersdk.Error
		require.ErrorAs(t, err, &sdkErr)
		require.Equal(t, http.StatusNotFound, sdkErr.StatusCode())
	})
}

func extractTarFiles(t *testing.T, data []byte) map[string]string {
	t.Helper()
	tr := tar.NewReader(bytes.NewReader(data))
	files := make(map[string]string)
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		require.NoError(t, err)
		body, err := io.ReadAll(tr)
		require.NoError(t, err)
		files[hdr.Name] = string(body)
	}
	return files
}
