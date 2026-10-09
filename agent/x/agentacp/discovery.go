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
	acp "github.com/coder/acp-go-sdk"
	"github.com/coder/coder/v2/buildinfo"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
)

const discoveryInterval = 30 * time.Second

type harnessConfig struct {
	slug, displayName, command string
	hash                       [32]byte
	directory                  string
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
	for i := range out {
		out[i] = cloneHarness(out[i])
	}
	return out
}

// Reload discovers immediate configuration files and probes changed harnesses.
// It never prompts a harness. Periodic discovery calls this independently of
// workspace readiness and context resolution reads only the cached results.
func (m *Manager) Reload(ctx context.Context) error {
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return ErrUnavailable
	}
	m.wg.Add(1)
	m.mu.Unlock()
	defer m.wg.Done()
	lifetime, cancel := context.WithCancel(m.ctx)
	stop := context.AfterFunc(ctx, cancel)
	defer stop()
	defer cancel()
	return m.discover(lifetime)
}

// RefreshDiscovery schedules a reload after discovery has started.
func (m *Manager) RefreshDiscovery() {
	select {
	case m.reloadTrigger <- struct{}{}:
	default:
	}
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
	m.mu.Lock()
	previous := m.configs
	cached := make(map[string]workspacesdk.ACPHarness)
	for _, h := range m.catalog {
		cached[h.Slug] = h
	}
	m.mu.Unlock()
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
		h := workspacesdk.ACPHarness{Slug: slug, DisplayName: slug, ConfigOptions: []workspacesdk.ACPConfigOption{}}
		raw, readErr := afero.ReadFile(m.fs, filepath.Join(configDir, entry.Name()))
		hashes[slug] = sha256.Sum256(raw)
		var file configFile
		switch {
		case readErr != nil:
			h.Error = "cannot read harness configuration"
		case json.Unmarshal(raw, &file) != nil:
			h.Error = "invalid harness configuration JSON"
		case file.Enabled != nil && !*file.Enabled:
			continue
		case strings.TrimSpace(file.Command) == "":
			h.Error = "harness command is required"
		}
		if h.Error != "" {
			catalog = append(catalog, h)
			continue
		}
		if file.DisplayName != "" {
			h.DisplayName = file.DisplayName
		}
		canonical, _ := json.Marshal(file)
		cfg := harnessConfig{slug: slug, displayName: h.DisplayName, command: file.Command, hash: sha256.Sum256(canonical), directory: m.workingDir()}
		configs[slug] = cfg
		hashes[slug] = cfg.hash
		if old, ok := previous[slug]; ok && old == cfg {
			h = cached[slug]
		} else {
			h = m.probe(ctx, cfg)
		}
		catalog = append(catalog, h)
	}
	slices.SortFunc(catalog, func(a, b workspacesdk.ACPHarness) int { return strings.Compare(a.Slug, b.Slug) })
	m.mu.Lock()
	before, _ := json.Marshal(m.catalog)
	after, _ := json.Marshal(catalog)
	changed := string(before) != string(after) || !equalConfigs(m.configs, configs) || !maps.Equal(m.configHashes, hashes) || m.configDir != configDir
	m.configs, m.catalog, m.configHashes = configs, catalog, hashes
	m.configDir = configDir
	callback := m.onChange
	m.mu.Unlock()
	if changed && callback != nil {
		callback()
	}
	return nil
}

func equalConfigs(a, b map[string]harnessConfig) bool {
	if len(a) != len(b) {
		return false
	}
	for slug, cfg := range a {
		if b[slug] != cfg {
			return false
		}
	}
	return true
}

func (m *Manager) probe(ctx context.Context, cfg harnessConfig) workspacesdk.ACPHarness {
	h := workspacesdk.ACPHarness{Slug: cfg.slug, DisplayName: cfg.displayName, ConfigOptions: []workspacesdk.ACPConfigOption{}}
	if err := m.validate(cfg.slug, cfg.directory); err != nil {
		h.Error = err.Error()
		return h
	}
	ctx, cancel := m.timeout(ctx, setupTimeout)
	defer cancel()
	p, err := m.launch(ctx, cfg.directory, cfg, &client{})
	if err != nil {
		h.Error = "cannot start harness"
		return h
	}
	defer p.close()
	init, err := p.conn.Initialize(ctx, acp.InitializeRequest{ProtocolVersion: acp.ProtocolVersionNumber, ClientInfo: &acp.Implementation{Name: "coder-agent", Version: buildinfo.Version()}})
	if err != nil {
		h.Error = p.failure(err, cfg).Error()
		return h
	}
	if init.ProtocolVersion != acp.ProtocolVersionNumber {
		h.Error = "unsupported ACP protocol version"
		return h
	}
	h = harnessInfo(cfg, init)
	created, err := p.conn.NewSession(ctx, acp.NewSessionRequest{Cwd: cfg.directory, McpServers: []acp.McpServer{}})
	if err != nil {
		h.Error = p.failure(err, cfg).Error()
		return h
	}
	h.ConfigOptions = configOptions(created.ConfigOptions)
	return h
}

// RunDiscovery starts discovery once and returns without waiting for probes.
// It scans immediately and polls for configuration changes every 30 seconds.
func (m *Manager) RunDiscovery() {
	m.runOnce.Do(func() { m.run(m.pollDiscovery) })
}

func (m *Manager) pollDiscovery() {
	reload := func() {
		if err := m.Reload(m.ctx); err != nil {
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
		case <-m.reloadTrigger:
			reload()
		case <-timer.C:
			reload()
			timer.Reset(discoveryInterval, "acp-discovery")
		}
	}
}
