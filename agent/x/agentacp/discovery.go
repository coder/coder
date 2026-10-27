package agentacp

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"maps"
	"os"
	"path/filepath"
	"reflect"
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
	for i := range out {
		out[i] = cloneHarness(out[i])
	}
	return out
}

// Reload discovers immediate configuration files and probes changed harnesses.
// Failed probes retry on each reload; successful probes stay cached until their
// configuration, directory, or environment changes. It never prompts a harness.
// Periodic discovery calls this independently of workspace readiness and context
// resolution reads only the cached results.
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
	// Keep the cache key and all probes on the same snapshot, even if a
	// manifest update changes the callback's output during discovery.
	env := m.envInfo.Environ()
	if m.updateEnv != nil {
		env, err = m.updateEnv(env)
		if err != nil {
			return xerrors.Errorf("prepare ACP environment: %w", err)
		}
	}
	env = normalizeEnv(env)
	m.mu.Lock()
	previous := m.configs
	previousDirectory := m.discoveryDir
	sameEnvironment := slices.Equal(m.catalogEnv, env)
	cached := make(map[string]workspacesdk.ACPHarness)
	for _, h := range m.catalog {
		cached[h.Slug] = h
	}
	m.mu.Unlock()
	directory := m.workingDir()
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
		h := newHarness(slug, slug)
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
			if old, ok := previous[slug]; ok && old == cfg && previousDirectory == directory && sameEnvironment && cached[slug].Error == "" {
				h = cached[slug]
			} else {
				h = m.probe(ctx, cfg, directory, env)
			}
		}
	appendHarness:
		catalog = append(catalog, h)
	}
	slices.SortFunc(catalog, func(a, b workspacesdk.ACPHarness) int { return strings.Compare(a.Slug, b.Slug) })
	m.mu.Lock()
	changed := !reflect.DeepEqual(m.catalog, catalog) || !maps.Equal(m.configs, configs) || !maps.Equal(m.configHashes, hashes) || m.configDir != configDir || m.discoveryDir != directory
	m.configs, m.catalog, m.configHashes = configs, catalog, hashes
	m.configDir = configDir
	m.discoveryDir = directory
	m.catalogEnv = env
	callback := m.onChange
	m.mu.Unlock()
	if changed && callback != nil {
		callback()
	}
	return nil
}

// normalizeEnv copies and deduplicates the environment, keeping the last value
// for each variable. It sorts the result so cache comparisons do not depend on
// the callback's output order.
func normalizeEnv(env []string) []string {
	valuesByName := make(map[string]string, len(env))
	for _, entry := range env {
		if name, value, ok := strings.Cut(entry, "="); ok {
			valuesByName[name] = value
		}
	}
	updated := make([]string, 0, len(valuesByName))
	for _, name := range slices.Sorted(maps.Keys(valuesByName)) {
		updated = append(updated, name+"="+valuesByName[name])
	}
	return updated
}

func (m *Manager) probe(ctx context.Context, cfg harnessConfig, directory string, env []string) workspacesdk.ACPHarness {
	h := newHarness(cfg.slug, cfg.displayName)
	if err := m.validate(directory); err != nil {
		h.Error = err.Error()
		return h
	}
	ctx, cancel := timeout(ctx, m.clock, setupTimeout)
	defer cancel()
	p, err := launch(ctx, cfg, directory, &discoveryClient{}, launchOptions{
		clock:   m.clock,
		execer:  m.execer,
		envInfo: m.envInfo,
		env:     env,
	})
	if err != nil {
		h.Error = "cannot start harness"
		m.logger.Warn(ctx, h.Error, slog.F("harness", cfg.slug), slog.Error(err))
		return h
	}
	defer func() {
		// Drain stderr before logging. Raw diagnostics may contain launch
		// credentials, so keep them out of the catalog and workspace context.
		p.close()
		diagnostic := p.stderr.String()
		if h.Error != "" {
			m.logger.Warn(ctx, h.Error, slog.F("harness", cfg.slug), slog.Error(err), slog.F("stderr", diagnostic))
		} else if diagnostic != "" {
			m.logger.Debug(ctx, "acp harness diagnostics", slog.F("harness", cfg.slug), slog.F("stderr", diagnostic))
		}
	}()
	init, err := p.conn.Initialize(ctx, acp.InitializeRequest{ProtocolVersion: acp.ProtocolVersionNumber, ClientInfo: &acp.Implementation{Name: "coder-agent", Version: buildinfo.Version()}})
	if err != nil {
		h.Error = "cannot initialize harness; see workspace agent logs for details"
		return h
	}
	if init.ProtocolVersion != acp.ProtocolVersionNumber {
		h.Error = "unsupported ACP protocol version"
		return h
	}
	h = harnessInfo(cfg, init)
	created, err := p.conn.NewSession(ctx, acp.NewSessionRequest{Cwd: directory, McpServers: []acp.McpServer{}})
	if err != nil {
		h.Error = "cannot create harness session; see workspace agent logs for details"
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
