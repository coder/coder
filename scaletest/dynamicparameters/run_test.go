package dynamicparameters_test

import (
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3"

	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/scaletest/dynamicparameters"
	"github.com/coder/coder/v2/testutil"
)

func TestRun(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitLong)

	client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
	client.SetLogger(testutil.Logger(t).Leveled(slog.LevelDebug))
	first := coderdtest.CreateFirstUser(t, client)
	userClient, _ := coderdtest.CreateAnotherUser(t, client, first.OrganizationID)
	orgID := first.OrganizationID

	dynamicParametersTerraformSource, err := dynamicparameters.TemplateContent()
	require.NoError(t, err)

	template, version := coderdtest.DynamicParameterTemplate(t, client, orgID, coderdtest.DynamicParameterTemplateParams{
		MainTF:         dynamicParametersTerraformSource,
		Plan:           nil,
		ModulesArchive: nil,
		StaticParams:   nil,
		ExtraFiles:     dynamicparameters.GetModuleFiles(),
	})

	reg := prometheus.NewRegistry()
	cfg := dynamicparameters.Config{
		TemplateVersion:   version.ID,
		Metrics:           dynamicparameters.NewMetrics(reg, "template", "test_label_name"),
		MetricLabelValues: []string{template.Name, "test_label_value"},
	}
	runner := dynamicparameters.NewRunner(userClient, cfg)
	var logs strings.Builder
	err = runner.Run(ctx, t.Name(), &logs)
	t.Log("Runner logs:\n\n" + logs.String())
	require.NoError(t, err)
}
