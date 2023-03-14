package terraform_test

import (
	"context"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hashicorp/go-version"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/sloggers/slogtest"
	"github.com/coder/coder/provisioner/terraform"
)

func TestInstall(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.SkipNow()
	}
	ctx := context.Background()
	dir := t.TempDir()
	log := slogtest.Make(t, nil)

	// install spins off 8 installs with Version and waits for them all
	// to complete.
	install := func(version *version.Version) string {
		var wg sync.WaitGroup
		var path atomic.Pointer[string]
		for i := 0; i < 8; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				p, err := terraform.Install(ctx, log, dir, version)
				assert.NoError(t, err)
				path.Store(&p)
			}()
		}
		wg.Wait()
		if t.Failed() {
			t.FailNow()
		}
		return *path.Load()
	}

	binPath := install(terraform.TerraformVersion)

	checkBin := func() time.Time {
		binInfo, err := os.Stat(binPath)
		require.NoError(t, err)
		require.Greater(t, binInfo.Size(), int64(0))
		return binInfo.ModTime()
	}

	firstMod := checkBin()

	// Since we're using the same version the install should be idempotent.
	install(terraform.TerraformVersion)
	secondMod := checkBin()
	require.Equal(t, firstMod, secondMod)

	// Ensure a new install happens when version changes
	differentVersion := version.Must(version.NewVersion("1.2.0"))
	// Sanity-check
	require.NotEqual(t, differentVersion.String(), terraform.TerraformVersion.String())

	install(differentVersion)

	thirdMod := checkBin()
	require.Greater(t, thirdMod, secondMod)
}
