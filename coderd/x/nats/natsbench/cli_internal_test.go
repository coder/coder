package natsbench

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/testutil"
)

func TestCLIScenarios(t *testing.T) {
	t.Parallel()

	t.Run("DefaultMatrix", func(t *testing.T) {
		t.Parallel()
		got, err := cliRun{timeout: testutil.WaitShort}.scenarios()
		require.NoError(t, err)
		require.Len(t, got, len(DefaultScenarios()))
		for _, sc := range got {
			require.Equal(t, testutil.WaitShort, sc.Config.Timeout)
		}
	})

	t.Run("MessageOverride", func(t *testing.T) {
		t.Parallel()
		got, err := cliRun{messages: 5, timeout: testutil.WaitShort}.scenarios()
		require.NoError(t, err)
		for _, sc := range got {
			require.Equal(t, 5, sc.Config.Messages)
		}
	})

	t.Run("NamedScenario", func(t *testing.T) {
		t.Parallel()
		got, err := cliRun{scenarioName: "8KiB-1r", messages: 9, timeout: testutil.WaitShort}.scenarios()
		require.NoError(t, err)
		require.Len(t, got, 1)
		require.Equal(t, "8KiB-1r", got[0].Name)
		require.Equal(t, 9, got[0].Config.Messages)
	})

	t.Run("UnknownScenario", func(t *testing.T) {
		t.Parallel()
		_, err := cliRun{scenarioName: "nope", timeout: testutil.WaitShort}.scenarios()
		require.Error(t, err)
	})

	t.Run("CustomShape", func(t *testing.T) {
		t.Parallel()
		got, err := cliRun{
			shapeFlagSet: true,
			payload:      Payload64KB,
			subjects:     3,
			publishers:   4,
			subscribers:  8,
			replicas:     2,
			timeout:      testutil.WaitShort,
		}.scenarios()
		require.NoError(t, err)
		require.Len(t, got, 1)
		require.Equal(t, "custom", got[0].Name)
		// Custom runs default to the standard message count.
		require.Equal(t, DefaultMessages, got[0].Config.Messages)
		require.Equal(t, Payload64KB, got[0].Config.PayloadSize)
		require.Equal(t, 2, got[0].Config.Replicas)
	})

	t.Run("ScenarioAndShapeConflict", func(t *testing.T) {
		t.Parallel()
		_, err := cliRun{scenarioName: "8KiB-1r", shapeFlagSet: true, timeout: testutil.WaitShort}.scenarios()
		require.Error(t, err)
	})
}
