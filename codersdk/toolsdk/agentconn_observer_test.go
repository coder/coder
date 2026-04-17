package toolsdk_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/codersdk/toolsdk"
)

func TestWithAgentConnObserver_WiresHook(t *testing.T) {
	t.Parallel()

	var (
		dialCount    int
		releaseCount int
	)
	deps, err := toolsdk.NewDeps(nil, toolsdk.WithAgentConnObserver(func() func() {
		dialCount++
		return func() { releaseCount++ }
	}))
	require.NoError(t, err)
	require.True(t, deps.HasAgentConnObserver(), "option must populate the observer")

	release, ok := deps.InvokeAgentConnObserver()
	require.True(t, ok)
	require.NotNil(t, release)
	assert.Equal(t, 1, dialCount)
	assert.Equal(t, 0, releaseCount)

	release()
	assert.Equal(t, 1, releaseCount)
}

func TestWithAgentConnObserver_Unset(t *testing.T) {
	t.Parallel()

	deps, err := toolsdk.NewDeps(nil)
	require.NoError(t, err)
	assert.False(t, deps.HasAgentConnObserver())
}
