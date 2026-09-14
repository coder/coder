package tz_test

import (
	"os"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/util/tz"
)

//nolint:paralleltest // Environment variables
func Test_TimezoneIANA(t *testing.T) {
	//nolint:paralleltest // t.Setenv
	t.Run("Env", func(t *testing.T) {
		t.Setenv("TZ", "Europe/Dublin")

		zone, err := tz.TimezoneIANA()
		assert.NoError(t, err)
		if assert.NotNil(t, zone) {
			assert.Equal(t, "Europe/Dublin", zone.String())
		}
	})

	//nolint:paralleltest // UnsetEnv
	t.Run("NoEnv", func(t *testing.T) {
		_, err := os.Stat("/etc/localtime")
		if runtime.GOOS == "linux" && err != nil {
			// Not all Linux operating systems are guaranteed to have localtime!
			t.Skip("localtime doesn't exist!")
		}
		if runtime.GOOS == "windows" {
			// This test can be flaky on some Windows runners :(
			t.Skip("This test is flaky under Windows.")
		}
		_, found := os.LookupEnv("TZ")
		if found {
			require.NoError(t, os.Unsetenv("TZ"))
		}

		zone, err := tz.TimezoneIANA()
		assert.NoError(t, err)
		assert.NotNil(t, zone)
	})
}
