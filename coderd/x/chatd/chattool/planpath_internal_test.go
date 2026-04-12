package chattool

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLooksLikePlanFileName(t *testing.T) {
	t.Parallel()

	require.True(t, looksLikePlanFileName(`C:\Users\coder\PLAN.md`))
	require.True(t, looksLikePlanFileName(`C:\Users\coder\plan.md`))
	require.False(t, looksLikePlanFileName(`C:\Users\coder\README.md`))
}
