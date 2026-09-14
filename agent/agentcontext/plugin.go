package agentcontext

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"unicode/utf8"

	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
)

// Agent Plugins (agent-plugins.org) manifest conventions.
const (
	// pluginManifestFileName sits at the plugin root.
	pluginManifestFileName = "plugin.json"
	// pluginMCPConfigFileName sits at the plugin root and declares
	// the plugin's MCP servers.
	pluginMCPConfigFileName = "mcp.json"
	// pluginSkillsDirName is the fixed skills container inside a
	// plugin root.
	pluginSkillsDirName = "skills"
	// pluginSchemaPrefix identifies a plugin.json that belongs to the
	// Agent Plugins format. Files whose $schema lacks this prefix are
	// some other tool's manifest and are ignored.
	pluginSchemaPrefix = "https://agent-plugins.org/schemas/"
	// PluginSchemaV1 is the only $schema value the resolver accepts.
	PluginSchemaV1 = pluginSchemaPrefix + "1.0.0/plugin.schema.json"

	// MaxPluginVersionRunes caps the manifest version carried on a
	// plugin resource.
	MaxPluginVersionRunes = 64
	// MaxPluginDescriptionRunes caps the manifest description carried
	// on a plugin resource.
	MaxPluginDescriptionRunes = 1024
)

// pluginContainerRelPaths are the directories, relative to a scan
// root, whose immediate subdirectories are plugin candidates.
var pluginContainerRelPaths = []string{
	"plugins",
	filepath.Join(".agents", "plugins"),
}

// pluginManifestFields are the top-level keys the 1.0.0 manifest
// schema defines. Any other key is reported and ignored.
var pluginManifestFields = map[string]struct{}{
	"$schema":     {},
	"name":        {},
	"version":     {},
	"description": {},
	"author":      {},
	"homepage":    {},
	"repository":  {},
	"license":     {},
	"keywords":    {},
	"extensions":  {},
}

// pluginAuthorFields are the keys allowed inside the author object.
// The object is closed: any other key rejects the manifest.
var pluginAuthorFields = map[string]struct{}{
	"name":  {},
	"email": {},
	"url":   {},
}

// pluginManifest is the validated content of a plugin.json.
type pluginManifest struct {
	Name        string
	Version     string
	Description string
	// Warnings lists the non-fatal deviations the spec requires a
	// client to report and ignore: unknown top-level fields and a
	// non-object extensions value.
	Warnings []string
}

// parsePluginManifest validates data as an Agent Plugins 1.0.0
// manifest. recognized is false when the document is not a JSON object
// or its $schema is absent or names a different format; such files
// belong to some other tool and produce no resource. When recognized
// is true, a non-nil err rejects the plugin.
func parsePluginManifest(data []byte) (m pluginManifest, recognized bool, err error) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil || fields == nil {
		return pluginManifest{}, false, nil
	}

	var schema string
	if raw, ok := fields["$schema"]; !ok || json.Unmarshal(raw, &schema) != nil || !strings.HasPrefix(schema, pluginSchemaPrefix) {
		return pluginManifest{}, false, nil
	}
	if schema != PluginSchemaV1 {
		return pluginManifest{}, true, xerrors.Errorf("unsupported schema version %q", schema)
	}

	rawName, ok := fields["name"]
	if !ok {
		return pluginManifest{}, true, xerrors.New("name is required")
	}
	if err := json.Unmarshal(rawName, &m.Name); err != nil {
		return pluginManifest{}, true, xerrors.New("name must be a string")
	}
	if err := workspacesdk.ValidatePluginName(m.Name); err != nil {
		return pluginManifest{}, true, err
	}

	for _, key := range []string{"version", "description", "homepage", "repository", "license"} {
		raw, ok := fields[key]
		if !ok {
			continue
		}
		var value string
		if err := json.Unmarshal(raw, &value); err != nil {
			return pluginManifest{}, true, xerrors.Errorf("%s must be a string", key)
		}
		switch key {
		case "version":
			m.Version = value
		case "description":
			m.Description = value
		}
	}

	if raw, ok := fields["keywords"]; ok {
		var keywords []string
		if err := json.Unmarshal(raw, &keywords); err != nil || keywords == nil {
			return pluginManifest{}, true, xerrors.New("keywords must be an array of strings")
		}
	}

	if raw, ok := fields["author"]; ok {
		var author map[string]json.RawMessage
		if err := json.Unmarshal(raw, &author); err != nil || author == nil {
			return pluginManifest{}, true, xerrors.New("author must be an object")
		}
		for key, value := range author {
			if _, known := pluginAuthorFields[key]; !known {
				return pluginManifest{}, true, xerrors.Errorf("author contains unknown field %q", key)
			}
			var s string
			if err := json.Unmarshal(value, &s); err != nil {
				return pluginManifest{}, true, xerrors.Errorf("author.%s must be a string", key)
			}
		}
	}

	if raw, ok := fields["extensions"]; ok {
		trimmed := strings.TrimSpace(string(raw))
		if !strings.HasPrefix(trimmed, "{") {
			m.Warnings = append(m.Warnings, "extensions must be an object; ignored")
		}
	}

	var unknown []string
	for key := range fields {
		if _, known := pluginManifestFields[key]; !known {
			unknown = append(unknown, key)
		}
	}
	slices.Sort(unknown)
	for _, key := range unknown {
		m.Warnings = append(m.Warnings, fmt.Sprintf("unknown field %q ignored", key))
	}

	m.Version = truncateRunes(codersdk.SanitizePromptText(m.Version), MaxPluginVersionRunes)
	m.Description = truncateRunes(codersdk.SanitizePromptText(m.Description), MaxPluginDescriptionRunes)
	return m, true, nil
}

// truncateRunes returns s cut to at most limit runes.
func truncateRunes(s string, limit int) string {
	if utf8.RuneCountInString(s) <= limit {
		return s
	}
	runes := []rune(s)
	return string(runes[:limit])
}

// pluginContainersFor returns the existing plugin-container
// directories directly under rootPath.
func pluginContainersFor(rootPath string) []string {
	var out []string
	for _, rel := range pluginContainerRelPaths {
		container := filepath.Join(rootPath, rel)
		if info, err := os.Stat(container); err == nil && info.IsDir() {
			out = append(out, container)
		}
	}
	return out
}

// discoverPlugin inspects dir as a plugin root. It returns false when
// dir holds no Agent Plugins manifest, in which case nothing is
// emitted. Otherwise it emits one KindPlugin resource and, when the
// manifest validates and the plugin name is not already claimed in
// pluginNames, one KindSkill resource per skill under skills/. The
// returned bool reports whether the plugin resource is StatusOK.
//
// Every path read inside the plugin is checked against the plugin
// root after symlink resolution. The root is canonicalized once so
// prefix checks, resource IDs, and Source values all use the same
// path.
func (r *Resolver) discoverPlugin(dir string, root ScanRoot, out *[]Resource, seenID map[string]int, pluginNames map[string]int) (recognized, ok bool) {
	pluginRoot, err := filepath.EvalSymlinks(dir)
	if err != nil {
		return false, false
	}
	manifestPath := filepath.Join(pluginRoot, pluginManifestFileName)
	info, err := os.Lstat(manifestPath)
	if err != nil {
		return false, false
	}

	res := Resource{
		ID:         resourceID(KindPlugin, pluginRoot),
		Kind:       KindPlugin,
		Source:     pluginRoot,
		SizeBytes:  safeUint64(info.Size()),
		SourcePath: root.UserSource,
	}
	readPath, readInfo, resolved, status, errMsg := resolveReadTarget(manifestPath, info, pluginRoot)
	if !resolved {
		res.Status = status
		res.Error = errMsg
		appendResource(out, seenID, res)
		return true, false
	}
	if !readInfo.Mode().IsRegular() {
		return false, false
	}
	res.SizeBytes = safeUint64(readInfo.Size())
	if res.SizeBytes > r.MaxResourceBytes {
		res.Status = StatusOversize
		res.Error = fmt.Sprintf("file size %d exceeds per-resource cap of %d bytes", readInfo.Size(), r.MaxResourceBytes)
		if data, err := readFileCapped(readPath, safeInt64(r.MaxResourceBytes)); err == nil {
			res.ContentHash = sha256.Sum256(data)
		}
		appendResource(out, seenID, res)
		return true, false
	}
	data, err := os.ReadFile(readPath)
	if err != nil {
		res.Status = StatusUnreadable
		res.Error = err.Error()
		appendResource(out, seenID, res)
		return true, false
	}
	res.ContentHash = sha256.Sum256(data)

	manifest, recognized, err := parsePluginManifest(data)
	if !recognized {
		return false, false
	}
	if err != nil {
		res.Status = StatusInvalid
		res.Error = err.Error()
		appendResource(out, seenID, res)
		return true, false
	}
	if _, taken := pluginNames[manifest.Name]; taken {
		res.Status = StatusInvalid
		res.Error = fmt.Sprintf("duplicate plugin name %q", manifest.Name)
		appendResource(out, seenID, res)
		return true, false
	}
	// The claim order is the scan-root priority order, which
	// Snapshot.Plugins later restores after resources are sorted by ID.
	res.pluginOrder = len(pluginNames)
	pluginNames[manifest.Name] = res.pluginOrder

	res.Name = manifest.Name
	res.PluginName = manifest.Name
	res.PluginVersion = manifest.Version
	res.Description = manifest.Description
	warnings := manifest.Warnings

	if hasMCP, warn := r.pluginMCPConfig(pluginRoot); warn != "" {
		warnings = append(warnings, warn)
	} else {
		res.HasMCPConfig = hasMCP
	}

	skills, warn := r.pluginSkills(pluginRoot, root, manifest.Name)
	if warn != "" {
		warnings = append(warnings, warn)
	}

	res.Error = strings.Join(warnings, "; ")
	appendResource(out, seenID, res)
	for _, skill := range skills {
		appendResource(out, seenID, skill)
	}
	return true, true
}

// pluginMCPConfig reports whether pluginRoot holds an mcp.json that
// resolves to a regular file inside the plugin root. A symlinked
// mcp.json that escapes the root or is not a regular file yields a
// warning and false.
func (*Resolver) pluginMCPConfig(pluginRoot string) (bool, string) {
	path := filepath.Join(pluginRoot, pluginMCPConfigFileName)
	info, err := os.Lstat(path)
	if err != nil {
		return false, ""
	}
	_, readInfo, ok, _, errMsg := resolveReadTarget(path, info, pluginRoot)
	if !ok {
		return false, "mcp.json ignored: " + errMsg
	}
	if !readInfo.Mode().IsRegular() {
		return false, "mcp.json ignored: not a regular file"
	}
	return true, ""
}

// pluginSkills reads the skills/ container inside pluginRoot and
// returns one KindSkill resource per immediate child directory holding
// a SKILL.md. The container itself must be a real directory: a
// symlinked or non-directory skills entry disables the skills
// component and is returned as a warning. Symlinked skill directories
// are skipped; a symlinked SKILL.md must resolve inside pluginRoot.
func (r *Resolver) pluginSkills(pluginRoot string, root ScanRoot, pluginName string) ([]Resource, string) {
	container := filepath.Join(pluginRoot, pluginSkillsDirName)
	info, err := os.Lstat(container)
	if err != nil {
		return nil, ""
	}
	if info.Mode()&fs.ModeSymlink != 0 {
		return nil, "skills directory is a symlink; skills not loaded"
	}
	if !info.IsDir() {
		return nil, "skills is not a directory; skills not loaded"
	}
	entries, err := os.ReadDir(container)
	if err != nil {
		return nil, "skills directory unreadable: " + err.Error()
	}
	var skills []Resource
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		meta := filepath.Join(container, e.Name(), skillMetaFileName)
		metaInfo, err := os.Lstat(meta)
		if err != nil {
			continue
		}
		res, ok := r.readSkillMeta(pluginRoot, meta, metaInfo, root.UserSource)
		if !ok {
			continue
		}
		res.PluginName = pluginName
		skills = append(skills, res)
	}
	return skills, ""
}

// PluginInfo describes a validated plugin present in a Snapshot.
type PluginInfo struct {
	// Name is the manifest name.
	Name string
	// Root is the canonical plugin root directory.
	Root string
	// HasMCPConfig reports that Root holds a readable mcp.json.
	HasMCPConfig bool
}

// Plugins returns the StatusOK plugin resources in s as PluginInfo
// values, ordered by the scan root that produced them (user sources,
// then built-in roots, then the working directory) rather than by
// resource ID, so callers that resolve name collisions by position
// favor the higher-priority root.
func (s Snapshot) Plugins() []PluginInfo {
	var plugins []Resource
	for _, r := range s.Resources {
		if r.Kind != KindPlugin || r.Status != StatusOK {
			continue
		}
		plugins = append(plugins, r)
	}
	slices.SortStableFunc(plugins, func(a, b Resource) int {
		return a.pluginOrder - b.pluginOrder
	})
	out := make([]PluginInfo, 0, len(plugins))
	for _, r := range plugins {
		out = append(out, PluginInfo{
			Name:         r.Name,
			Root:         r.Source,
			HasMCPConfig: r.HasMCPConfig,
		})
	}
	return out
}
