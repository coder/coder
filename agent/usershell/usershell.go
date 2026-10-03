package usershell

import (
	"os"
	"os/user"

	"github.com/spf13/afero"
	"golang.org/x/xerrors"
)

// homeDir returns the home directory of the current user, giving
// priority to the $HOME environment variable. It backs
// SystemEnvInfo.HomeDir. Callers outside this package resolve the home
// directory through an EnvInfoer so the injected environment is honored.
func homeDir() (string, error) {
	// First we check the environment.
	homedir, err := os.UserHomeDir()
	if err == nil {
		return homedir, nil
	}

	// As a fallback, we try the user information.
	u, err := user.Current()
	if err != nil {
		return "", xerrors.Errorf("current user: %w", err)
	}
	return u.HomeDir, nil
}

// ResolveWorkingDirectory returns dir when it is non-empty and an existing
// directory on fs. Otherwise it falls back to the home directory
// reported by ei. SSH sessions and the process API share this so their
// working directory resolution cannot drift, and the home fallback goes
// through the injected EnvInfoer rather than the host directly.
func ResolveWorkingDirectory(fs afero.Fs, ei EnvInfoer, dir string) (string, error) {
	if dir != "" {
		if info, err := fs.Stat(dir); err == nil && info.IsDir() {
			return dir, nil
		}
	}
	return ei.HomeDir()
}

// EnvInfoer encapsulates external information about the environment.
type EnvInfoer interface {
	// User returns the current user.
	User() (*user.User, error)
	// Environ returns the environment variables of the current process.
	Environ() []string
	// HomeDir returns the home directory of the current user.
	HomeDir() (string, error)
	// Shell returns the shell of the given user.
	Shell(username string) (string, error)
	// ModifyCommand modifies the command and arguments before execution based on
	// the environment. This is useful for executing a command inside a container.
	// In the default case, the command and arguments are returned unchanged.
	ModifyCommand(name string, args ...string) (string, []string)
}

// SystemEnvInfo encapsulates the information about the environment
// just using the default Go implementations.
type SystemEnvInfo struct{}

func (SystemEnvInfo) User() (*user.User, error) {
	return user.Current()
}

func (SystemEnvInfo) Environ() []string {
	var env []string
	for _, e := range os.Environ() {
		// Ignore GOTRACEBACK=none, as it disables stack traces, it can
		// be set on the agent due to changes in capabilities.
		// https://pkg.go.dev/runtime#hdr-Security.
		if e == "GOTRACEBACK=none" {
			continue
		}
		env = append(env, e)
	}
	return env
}

func (SystemEnvInfo) HomeDir() (string, error) {
	return homeDir()
}

func (SystemEnvInfo) Shell(username string) (string, error) {
	return get(username)
}

func (SystemEnvInfo) ModifyCommand(name string, args ...string) (string, []string) {
	return name, args
}
