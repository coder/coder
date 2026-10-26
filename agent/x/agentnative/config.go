package agentnative

import (
	"encoding/json"
	"errors"
	"net/url"
	"os"
	"path/filepath"

	"golang.org/x/xerrors"
)

// Config contains workspace startup inputs, not native installation or credentials.
type Config struct {
	InitialJob *JobSpec `json:"initialJob,omitempty"`
}

// DefaultStateDir is separate from the previous proof-of-concept's state. Native
// managers never migrate, overwrite, or destroy that existing state.
func DefaultStateDir(home string) string {
	return filepath.Join(home, ".local", "state", "coder-native")
}

// ReadConfig reads private startup configuration. An absent file or directory
// means no initial job, not a request to run a default prompt.
func ReadConfig(stateDir string) (Config, error) {
	dir, err := openPrivateDirectory(stateDir, privateDirectoryExisting)
	if errors.Is(err, os.ErrNotExist) {
		return Config{}, nil
	}
	if err != nil {
		return Config{}, err
	}
	defer dir.close()
	data, _, err := dir.read("config.json", 1<<20)
	if errors.Is(err, os.ErrNotExist) {
		return Config{}, nil
	}
	if err != nil {
		return Config{}, err
	}
	var config Config
	if err := json.Unmarshal(data, &config); err != nil {
		return Config{}, xerrors.New("invalid native startup configuration")
	}
	if config.InitialJob != nil {
		spec, err := normalizeJob(*config.InitialJob)
		if err != nil {
			return Config{}, err
		}
		config.InitialJob = &spec
	}
	return config, nil
}

// WriteConfig atomically writes private configuration before manager startup.
// It refuses to race a running manager, and never rewrites a job's dispatch claim.
func WriteConfig(stateDir string, config Config) error {
	if config.InitialJob != nil {
		spec, err := normalizeJob(*config.InitialJob)
		if err != nil {
			return err
		}
		config.InitialJob = &spec
	}
	data, err := json.Marshal(config)
	if err != nil {
		return err
	}
	dir, err := openPrivateDirectory(stateDir, privateDirectoryCreate)
	if err != nil {
		return err
	}
	defer dir.close()
	if err := dir.acquire(); err != nil {
		return err
	}
	_, stamp, err := dir.read("config.json", 1<<20)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	_, err = dir.write("config.json", append(data, '\n'), stamp)
	return err
}

// Descriptor is a private local connection credential. Do not expose it through
// the remote agent API or log its bearer token.
type Descriptor struct {
	URL   string `json:"url"`
	Token string `json:"token"`
}

// ReadDescriptor securely reads the local manager endpoint for native CLI calls.
func ReadDescriptor(stateDir string) (Descriptor, error) {
	dir, err := openPrivateDirectory(stateDir, privateDirectoryExisting)
	if err != nil {
		return Descriptor{}, err
	}
	defer dir.close()
	data, _, err := dir.read("manager.json", 16384)
	if err != nil {
		return Descriptor{}, err
	}
	var descriptor Descriptor
	if err := json.Unmarshal(data, &descriptor); err != nil {
		return Descriptor{}, xerrors.New("invalid native manager descriptor")
	}
	u, err := url.Parse(descriptor.URL)
	if err != nil || u.Scheme != "http" || u.Hostname() != "127.0.0.1" || u.Port() == "" || u.User != nil || u.Path != "" || u.RawQuery != "" || u.Fragment != "" || len(descriptor.Token) < 32 {
		return Descriptor{}, xerrors.New("invalid native manager descriptor")
	}
	return descriptor, nil
}
