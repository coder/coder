package agentworkload

import (
	"context"
	"encoding/json"
	"io"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"

	"golang.org/x/xerrors"
)

const (
	runtimeOverhead = int64(64 << 20)
	maxHistory      = 1024
	maxOutputBytes  = 4 << 20
	maxOutputEvents = 4096
	maxBatchBytes   = 256 << 10
	maxBatchEvents  = 128
)

// Config describes a prepared host. Token must be generated randomly with at
// least 32 ASCII characters. Capacity excludes fixed agent and daemon overhead.
// DockerHost defaults to the local Unix socket and must be a Unix socket URL.
// No Docker environment variables or client credential files are read.
type Config struct {
	Token        string            `json:"token"`
	StateDir     string            `json:"state_dir"`
	Image        string            `json:"image"`
	Workdir      string            `json:"workdir"`
	Shell        string            `json:"shell"`
	Repositories map[string]string `json:"repositories"`
	CPU          int64             `json:"cpu"`
	MemoryBytes  int64             `json:"memory_bytes"`
	DockerHost   string            `json:"docker_host,omitempty"`
}

var commitPattern = regexp.MustCompile(`^(?:[a-fA-F0-9]{40}|[a-fA-F0-9]{64})$`)

func (c *Config) validate() error {
	if len(c.Token) < 32 || len(c.Token) > 4096 || strings.IndexFunc(c.Token, func(r rune) bool { return r < 33 || r > 126 }) >= 0 {
		return xerrors.New("token must contain 32 to 4096 randomly generated printable ASCII characters without spaces")
	}
	if !filepath.IsAbs(c.StateDir) || !filepath.IsAbs(c.Workdir) || strings.ContainsRune(c.Workdir, 0) {
		return xerrors.New("state_dir and workdir must be absolute paths")
	}
	if c.Image == "" || len(c.Image) > 512 {
		return xerrors.New("image must identify an already prepared local image")
	}
	if c.Shell == "" {
		c.Shell = "/bin/sh"
	}
	if !filepath.IsAbs(c.Shell) || strings.ContainsRune(c.Shell, 0) {
		return xerrors.New("shell must be an absolute path inside the image")
	}
	if c.CPU <= 0 || c.CPU > math.MaxInt64/1_000_000_000 || c.MemoryBytes < runtimeOverhead+(16<<20) {
		return xerrors.New("cpu and memory_bytes must specify positive allocatable capacity including per-run overhead")
	}
	if len(c.Repositories) == 0 || len(c.Repositories) > 128 {
		return xerrors.New("repositories must declare 1 to 128 prepared repository commits")
	}
	for name, commit := range c.Repositories {
		if name == "" || len(name) > 1024 || !commitPattern.MatchString(commit) {
			return xerrors.New("repositories must map nonempty names to full hexadecimal commit IDs")
		}
	}
	if c.DockerHost == "" {
		c.DockerHost = "unix:///var/run/docker.sock"
	}
	if !strings.HasPrefix(c.DockerHost, "unix:///") || strings.ContainsAny(c.DockerHost, "\x00\r\n") {
		return xerrors.New("docker_host must be an absolute local Unix socket URL")
	}
	return nil
}

// NewFromFile loads a private (0600) JSON config, validates the prepared Docker
// profile, and acquires the private durable state. ctx controls initialization
// only. Call Close to detach supervision. Outstanding state fails closed.
func NewFromFile(ctx context.Context, path string) (*Supervisor, error) {
	if runtime.GOOS != "linux" {
		return nil, xerrors.New("workload supervisor requires Linux")
	}
	file, err := readPrivateFile(path, 1<<20)
	if err != nil {
		return nil, xerrors.New("read private workload config: require a regular owner-only file")
	}
	var cfg Config
	if err := decodeJSON(strings.NewReader(string(file)), &cfg); err != nil {
		return nil, xerrors.New("invalid workload config JSON")
	}
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	if err := checkOuterAllocation(cfg); err != nil {
		return nil, err
	}
	rt, err := newDockerRuntime(cfg)
	if err != nil {
		return nil, xerrors.New("initialize Docker client")
	}
	s, err := newSupervisor(ctx, cfg, rt)
	if err != nil {
		_ = rt.Close()
	}
	return s, err
}

func decodeJSON(r io.Reader, out any) error {
	dec := json.NewDecoder(r)
	dec.DisallowUnknownFields()
	if err := dec.Decode(out); err != nil {
		return err
	}
	if err := dec.Decode(new(any)); err != io.EOF {
		return xerrors.New("expected a single JSON object")
	}
	return nil
}

func readPrivateFile(path string, limit int64) ([]byte, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0o077 != 0 || !privateOwner(info) || info.Size() > limit {
		return nil, xerrors.New("not a bounded private regular file")
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	opened, err := f.Stat()
	if err != nil || !os.SameFile(info, opened) {
		return nil, xerrors.New("private file changed during open")
	}
	data, err := io.ReadAll(io.LimitReader(f, limit+1))
	if err != nil || int64(len(data)) > limit {
		return nil, xerrors.New("read bounded private file")
	}
	return data, nil
}
