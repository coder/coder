package cli_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/coder/coder/cli/clitest"
)

// nolint:paralleltest
func TestDotfiles(t *testing.T) {
	t.Run("MissingArg", func(t *testing.T) {
		cmd, _ := clitest.New(t, "dotfiles")
		err := cmd.Execute()
		assert.Error(t, err)
	})
	t.Run("NoInstallScript", func(t *testing.T) {
		_, root := clitest.New(t)
		testRepo := filepath.Join(string(root), "test-repo")

		err := os.MkdirAll(testRepo, 0750)
		assert.NoError(t, err)

		c := exec.Command("git", "init")
		c.Dir = testRepo
		err = c.Run()
		assert.NoError(t, err)

		// nolint:gosec
		err = os.WriteFile(filepath.Join(testRepo, ".bashrc"), []byte("wow"), 0750)
		assert.NoError(t, err)

		c = exec.Command("git", "add", ".bashrc")
		c.Dir = testRepo
		err = c.Run()
		assert.NoError(t, err)

		c = exec.Command("git", "commit", "-m", `"add .bashrc"`)
		c.Dir = testRepo
		err = c.Run()
		assert.NoError(t, err)

		cmd, _ := clitest.New(t, "dotfiles", "--global-config", string(root), "--home-dir", string(root), "-y", testRepo)
		err = cmd.Execute()
		assert.NoError(t, err)

		b, err := os.ReadFile(filepath.Join(string(root), ".bashrc"))
		assert.NoError(t, err)
		assert.Equal(t, string(b), "wow")
	})
	t.Run("InstallScript", func(t *testing.T) {
		_, root := clitest.New(t)
		testRepo := filepath.Join(string(root), "test-repo")
		err := os.MkdirAll(testRepo, 0750)
		assert.NoError(t, err)
		c := exec.Command("git", "init")
		c.Dir = testRepo
		err = c.Run()
		assert.NoError(t, err)
		// nolint:gosec
		err = os.WriteFile(filepath.Join(testRepo, "install.sh"), []byte("#!/bin/bash\necho wow > "+filepath.Join(string(root), ".bashrc")), 0750)
		assert.NoError(t, err)
		c = exec.Command("git", "add", "install.sh")
		c.Dir = testRepo
		err = c.Run()
		assert.NoError(t, err)
		c = exec.Command("git", "commit", "-m", `"add install.sh"`)
		c.Dir = testRepo
		err = c.Run()
		assert.NoError(t, err)
		cmd, _ := clitest.New(t, "dotfiles", "--global-config", string(root), "--home-dir", string(root), "-y", testRepo)
		err = cmd.Execute()
		assert.NoError(t, err)
		b, err := os.ReadFile(filepath.Join(string(root), ".bashrc"))
		assert.NoError(t, err)
		assert.Equal(t, string(b), "wow\n")
	})
}
