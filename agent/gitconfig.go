package agent

import (
	"context"
	"os/exec"
	"os/user"
	"path/filepath"
	"strings"

	"golang.org/x/xerrors"
)

var errNoGitAvailable = xerrors.New("Git does not seem to be installed")

func setupGitconfig(ctx context.Context, configPath string, params map[string]string) error {
	if configPath == "" {
		return nil
	}
	if strings.HasPrefix(configPath, "~/") {
		currentUser, err := user.Current()
		if err != nil {
			return xerrors.Errorf("get current user: %w", err)
		}
		configPath = filepath.Join(currentUser.HomeDir, configPath[2:])
	}

	cmd := exec.CommandContext(ctx, "git", "--version")
	err := cmd.Run()
	if err != nil {
		return errNoGitAvailable
	}

	for name, value := range params {
		err = setGitConfigIfUnset(ctx, configPath, name, value)
		if err != nil {
			return err
		}
	}
	return nil
}

func setGitConfigIfUnset(ctx context.Context, configPath, name, value string) error {
	cmd := exec.CommandContext(ctx, "git", "config", "--file", configPath, "--get", name)
	err := cmd.Run()
	if err == nil {
		// an exit status of 0 means the value exists, so there's nothing to do
		return nil
	}
	// an exit status of 1 means the value is unset
	if cmd.ProcessState.ExitCode() != 1 {
		return xerrors.Errorf("getting %s: %w", name, err)
	}

	cmd = exec.CommandContext(ctx, "git", "config", "--file", configPath, "--add", name, value)
	_, err = cmd.Output()
	if err != nil {
		return xerrors.Errorf("setting %s=%s: %w", name, value, err)
	}
	return nil
}
