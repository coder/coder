package cli_test

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/clistat"
	"github.com/coder/coder/v2/cli/clitest"
	"github.com/coder/coder/v2/testutil"
)

// This just tests that the stat command is recognized and does not output
// an empty string. Actually testing the output of the stats command is
// fraught with all sorts of fun. Some more detailed testing of the stats
// output is performed in the tests in the clistat package.
func TestStatCmd(t *testing.T) {
	t.Parallel()
	t.Run("JSON", func(t *testing.T) {
		t.Parallel()
		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
		t.Cleanup(cancel)
		inv, _ := clitest.New(t, "stat", "all", "--output=json")
		buf := new(bytes.Buffer)
		inv.Stdout = buf
		err := inv.WithContext(ctx).Run()
		require.NoError(t, err)
		s := buf.String()
		require.NotEmpty(t, s)
		// Must be valid JSON
		tmp := make([]clistat.Result, 0)
		require.NoError(t, json.NewDecoder(strings.NewReader(s)).Decode(&tmp))
	})
	t.Run("Table", func(t *testing.T) {
		t.Parallel()
		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
		t.Cleanup(cancel)
		inv, _ := clitest.New(t, "stat", "all", "--output=table")
		buf := new(bytes.Buffer)
		inv.Stdout = buf
		err := inv.WithContext(ctx).Run()
		require.NoError(t, err)
		s := buf.String()
		require.NotEmpty(t, s)
		require.Contains(t, s, "HOST CPU")
		require.Contains(t, s, "HOST MEMORY")
		require.Contains(t, s, "HOME DISK")
	})
	t.Run("Default", func(t *testing.T) {
		t.Parallel()
		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
		t.Cleanup(cancel)
		inv, _ := clitest.New(t, "stat", "all")
		buf := new(bytes.Buffer)
		inv.Stdout = buf
		err := inv.WithContext(ctx).Run()
		require.NoError(t, err)
		s := buf.String()
		require.NotEmpty(t, s)
		require.Contains(t, s, "HOST CPU")
		require.Contains(t, s, "HOST MEMORY")
		require.Contains(t, s, "HOME DISK")
	})
}

func TestStatCPUCmd(t *testing.T) {
	t.Parallel()

	t.Run("Text", func(t *testing.T) {
		t.Parallel()
		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
		t.Cleanup(cancel)
		inv, _ := clitest.New(t, "stat", "cpu", "--output=text", "--host")
		buf := new(bytes.Buffer)
		inv.Stdout = buf
		err := inv.WithContext(ctx).Run()
		require.NoError(t, err)
		s := buf.String()
		require.NotEmpty(t, s)
	})

	t.Run("JSON", func(t *testing.T) {
		t.Parallel()
		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
		t.Cleanup(cancel)
		inv, _ := clitest.New(t, "stat", "cpu", "--output=json", "--host")
		buf := new(bytes.Buffer)
		inv.Stdout = buf
		err := inv.WithContext(ctx).Run()
		require.NoError(t, err)
		s := buf.String()
		tmp := clistat.Result{}
		require.NoError(t, json.NewDecoder(strings.NewReader(s)).Decode(&tmp))
		// require.NotZero(t, tmp.Used) // Host CPU can sometimes be zero.
		require.NotNil(t, tmp.Total)
		require.NotZero(t, *tmp.Total)
		require.Equal(t, "cores", tmp.Unit)
	})
}

func TestStatMemCmd(t *testing.T) {
	t.Parallel()

	t.Run("Text", func(t *testing.T) {
		t.Parallel()
		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
		t.Cleanup(cancel)
		inv, _ := clitest.New(t, "stat", "mem", "--output=text", "--host")
		buf := new(bytes.Buffer)
		inv.Stdout = buf
		err := inv.WithContext(ctx).Run()
		require.NoError(t, err)
		s := buf.String()
		require.NotEmpty(t, s)
	})

	t.Run("JSON", func(t *testing.T) {
		t.Parallel()
		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
		t.Cleanup(cancel)
		inv, _ := clitest.New(t, "stat", "mem", "--output=json", "--host")
		buf := new(bytes.Buffer)
		inv.Stdout = buf
		err := inv.WithContext(ctx).Run()
		require.NoError(t, err)
		s := buf.String()
		tmp := clistat.Result{}
		require.NoError(t, json.NewDecoder(strings.NewReader(s)).Decode(&tmp))
		require.NotZero(t, tmp.Used)
		require.NotNil(t, tmp.Total)
		require.NotZero(t, *tmp.Total)
		require.Equal(t, "B", tmp.Unit)
	})
}

func TestStatDiskCmd(t *testing.T) {
	t.Parallel()

	t.Run("Text", func(t *testing.T) {
		t.Parallel()
		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
		t.Cleanup(cancel)
		inv, _ := clitest.New(t, "stat", "disk", "--output=text")
		buf := new(bytes.Buffer)
		inv.Stdout = buf
		err := inv.WithContext(ctx).Run()
		require.NoError(t, err)
		s := buf.String()
		require.NotEmpty(t, s)
	})

	t.Run("JSON", func(t *testing.T) {
		t.Parallel()
		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
		t.Cleanup(cancel)
		inv, _ := clitest.New(t, "stat", "disk", "--output=json")
		buf := new(bytes.Buffer)
		inv.Stdout = buf
		err := inv.WithContext(ctx).Run()
		require.NoError(t, err)
		s := buf.String()
		tmp := clistat.Result{}
		require.NoError(t, json.NewDecoder(strings.NewReader(s)).Decode(&tmp))
		require.NotZero(t, tmp.Used)
		require.NotNil(t, tmp.Total)
		require.NotZero(t, *tmp.Total)
		require.Equal(t, "B", tmp.Unit)
	})

	t.Run("PosArg", func(t *testing.T) {
		t.Parallel()
		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
		t.Cleanup(cancel)
		inv, _ := clitest.New(t, "stat", "disk", "/this/path/does/not/exist", "--output=text")
		buf := new(bytes.Buffer)
		inv.Stdout = buf
		err := inv.WithContext(ctx).Run()
		require.Error(t, err)
		require.Contains(t, err.Error(), `not found: "/this/path/does/not/exist"`)
	})
}
