// Package sandbox implements disposable workspaces through a native runtime.
package sandbox

import (
	"archive/tar"
	"bytes"
	_ "crypto/sha256" // Register the supported image digest algorithm.
	"errors"
	"io"
	"math"
	"path"
	"strings"

	"github.com/distribution/reference"
	"github.com/opencontainers/go-digest"
	"golang.org/x/xerrors"
	"gopkg.in/yaml.v3"
)

const (
	// ManifestFilename is the native template entrypoint.
	ManifestFilename = "sandbox.yaml"
	// HostTag limits this prototype to a single explicitly configured host.
	HostTag = "sandbox_host"
	// HostID is the host supported by the single-host prototype.
	HostID          = "local"
	maxManifestSize = 64 << 10
)

// Manifest is an administrator-controlled, fixed sandbox template.
type Manifest struct {
	Version   int     `yaml:"version" json:"version"`
	Image     string  `yaml:"image" json:"image"`
	CPU       float64 `yaml:"cpu" json:"cpu"`
	MemoryMiB int64   `yaml:"memory_mib" json:"memory_mib"`
	Workdir   string  `yaml:"workdir" json:"workdir"`
	DailyCost int32   `yaml:"daily_cost" json:"daily_cost"`
}

// Tags returns the routing tags required by native sandbox templates.
func (Manifest) Tags() map[string]string { return map[string]string{HostTag: HostID} }

// ParseManifest validates a native template and supplies fixed defaults.
func ParseManifest(data []byte) (Manifest, error) {
	m := Manifest{CPU: 1, MemoryMiB: 2048, Workdir: "/workspace", DailyCost: 1}
	if len(data) > maxManifestSize {
		return m, xerrors.Errorf("sandbox manifest exceeds %d bytes", maxManifestSize)
	}
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	if err := decoder.Decode(&m); err != nil {
		return m, xerrors.Errorf("decode sandbox manifest: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return m, xerrors.New("sandbox manifest must contain one YAML document")
	}
	if m.Version != 1 {
		return m, xerrors.New("sandbox manifest version must be 1")
	}
	named, err := reference.ParseNormalizedNamed(m.Image)
	if err != nil {
		return m, xerrors.Errorf("invalid sandbox image reference: %w", err)
	}
	canonical, ok := named.(reference.Canonical)
	if !ok || canonical.Digest().Algorithm() != digest.SHA256 {
		return m, xerrors.New("sandbox image must be a named image pinned by sha256 digest")
	}
	if math.IsNaN(m.CPU) || math.IsInf(m.CPU, 0) || m.CPU < 0.1 || m.CPU > 64 {
		return m, xerrors.New("sandbox cpu must be between 0.1 and 64")
	}
	if m.MemoryMiB < 128 || m.MemoryMiB > 262144 {
		return m, xerrors.New("sandbox memory_mib must be between 128 and 262144")
	}
	if m.DailyCost < 1 {
		return m, xerrors.New("sandbox daily_cost must be positive")
	}
	if !path.IsAbs(m.Workdir) || path.Clean(m.Workdir) != m.Workdir || strings.ContainsAny(m.Workdir, "\x00\r\n") {
		return m, xerrors.New("sandbox workdir must be a clean absolute Linux path")
	}
	return m, nil
}

// ReadManifestArchive reads the manifest from a normalized template tar.
// It never extracts template-controlled paths onto the host filesystem.
func ReadManifestArchive(data []byte) (Manifest, error) {
	reader := tar.NewReader(bytes.NewReader(data))
	var result Manifest
	found := false
	for {
		header, err := reader.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return result, xerrors.Errorf("read sandbox archive: %w", err)
		}
		if strings.TrimPrefix(header.Name, "./") != ManifestFilename {
			continue
		}
		if found {
			return result, xerrors.New("duplicate sandbox manifest")
		}
		if header.Typeflag != tar.TypeReg || header.Size > maxManifestSize {
			return result, xerrors.Errorf("sandbox manifest must be a regular file of at most %d bytes", maxManifestSize)
		}
		body, err := io.ReadAll(io.LimitReader(reader, maxManifestSize+1))
		if err != nil {
			return result, xerrors.Errorf("read sandbox manifest: %w", err)
		}
		result, err = ParseManifest(body)
		if err != nil {
			return result, err
		}
		found = true
	}
	if !found {
		return result, xerrors.Errorf("template must contain %s at its root", ManifestFilename)
	}
	return result, nil
}
