package agentacp

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/spf13/afero"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
)

const (
	discoveryInterval = 30 * time.Second
	reloadTimeout     = 30 * time.Second
)

type harnessConfig struct {
	slug, displayName, command string
	hash                       [32]byte
}

type configFile struct {
	DisplayName string `json:"display_name"`
	Command     string `json:"command"`
	Enabled     *bool  `json:"enabled"`
}

// Catalog returns an immutable copy of the cached discovery results.
func (m *Manager) Catalog() []workspacesdk.ACPHarness {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.catalogLocked()
}

func (m *Manager) catalogLocked() []workspacesdk.ACPHarness {
	out := slices.Clone(m.catalog)
	if out == nil {
		out = []workspacesdk.ACPHarness{}
	}
	return out
}

// Reload discovers and validates immediate harness configuration files.
func (m *Manager) Reload(ctx context.Context) error {
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return ErrManagerClosed
	}
	m.wg.Add(1)
	m.mu.Unlock()
	defer m.wg.Done()
	return m.discover(ctx)
}

func (m *Manager) discover(ctx context.Context) error {
	m.reloadMu.Lock()
	defer m.reloadMu.Unlock()
	home, err := m.envInfo.HomeDir()
	if err != nil {
		return xerrors.Errorf("resolve ACP configuration directory: %w", err)
	}
	configDir := filepath.Join(home, ".coder", "acp")
	entries, err := afero.ReadDir(m.fs, configDir)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	configs := make(map[string]harnessConfig)
	hashes := make(map[string][32]byte)
	catalog := []workspacesdk.ACPHarness{}
	for _, entry := range entries {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if !entry.Mode().IsRegular() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		slug := strings.TrimSuffix(entry.Name(), ".json")
		if slug == "" {
			continue
		}
		h := workspacesdk.ACPHarness{Slug: slug, DisplayName: slug}
		{
			raw, err := afero.ReadFile(m.fs, filepath.Join(configDir, entry.Name()))
			hashes[slug] = sha256.Sum256(raw)
			if err != nil {
				h.Error = "cannot read harness configuration"
				goto appendHarness
			}
			var file configFile
			if err := json.Unmarshal(raw, &file); err != nil {
				h.Error = "invalid harness configuration JSON"
				goto appendHarness
			}
			if file.Enabled != nil && !*file.Enabled {
				continue
			}
			if strings.TrimSpace(file.Command) == "" {
				h.Error = "harness command is required"
				goto appendHarness
			}
			if file.DisplayName != "" {
				h.DisplayName = file.DisplayName
			}
			canonical, err := json.Marshal(file)
			if err != nil {
				return xerrors.Errorf("marshal ACP harness configuration %q: %w", slug, err)
			}
			cfg := harnessConfig{slug: slug, displayName: h.DisplayName, command: file.Command, hash: sha256.Sum256(canonical)}
			configs[slug] = cfg
			hashes[slug] = cfg.hash
		}
	appendHarness:
		catalog = append(catalog, h)
	}
	slices.SortFunc(catalog, func(a, b workspacesdk.ACPHarness) int { return strings.Compare(a.Slug, b.Slug) })
	m.mu.Lock()
	changed := !slices.Equal(m.catalog, catalog) || !maps.Equal(m.configs, configs) || !maps.Equal(m.configHashes, hashes) || m.configDir != configDir
	m.configs, m.catalog, m.configHashes = configs, catalog, hashes
	m.configDir = configDir
	callback := m.onChange
	m.mu.Unlock()
	if changed && callback != nil {
		callback()
	}
	return nil
}

// RunDiscovery starts filesystem discovery once and returns immediately.
// It scans immediately and polls for configuration changes every 30 seconds.
func (m *Manager) RunDiscovery() {
	m.runOnce.Do(func() { m.run(m.pollDiscovery) })
}

func (m *Manager) pollDiscovery() {
	reload := func() {
		ctx, cancel := context.WithTimeout(m.ctx, reloadTimeout)
		defer cancel()
		if err := m.Reload(ctx); err != nil {
			m.logger.Warn(m.ctx, "acp discovery failed", slog.Error(err))
		}
	}
	reload()
	timer := m.clock.NewTimer(discoveryInterval, "acp-discovery")
	defer timer.Stop()
	for {
		select {
		case <-m.ctx.Done():
			return
		case <-timer.C:
			reload()
			timer.Reset(discoveryInterval, "acp-discovery")
		}
	}
}
