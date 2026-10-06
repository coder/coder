package agentcontext

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"unicode/utf8"

	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	strutil "github.com/coder/coder/v2/coderd/util/strings"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
)

// Agent Plugins (agent-plugins.org) manifest conventions.
const (
	// pluginManifestFileName sits at the plugin root.
	pluginManifestFileName = "plugin.json"
	// pluginSkillsDirName is the fixed skills container inside a
	// plugin root.
	pluginSkillsDirName = "skills"
	// pluginSchemaPrefix identifies a plugin.json that belongs to the
	// Agent Plugins format. A file whose $schema lacks this prefix is
	// ignored at a scan root and in a plugins/ container, and reported
	// as invalid in a .agents/plugins/ container.
	pluginSchemaPrefix = "https://agent-plugins.org/schemas/"
	// PluginSchemaV1 is the only $schema value the resolver accepts.
	PluginSchemaV1 = pluginSchemaPrefix + "1.0.0/plugin.schema.json"

	// The spec limits name to 1 to 64 characters (section 5.5,
	// workspacesdk.MaxPluginNameLength) and gives version and
	// description no length limit (section 5.4). The version and
	// description caps below are Coder's own.

	// MaxPluginVersionRunes caps the manifest version carried on a
	// plugin resource.
	MaxPluginVersionRunes = 64
	// MaxPluginDescriptionRunes caps the manifest description carried
	// on a plugin resource.
	MaxPluginDescriptionRunes = 1024
)

// maxReportedUnknownFields bounds how many unknown manifest keys are
// named in a plugin's warnings.
const maxReportedUnknownFields = 10

// pluginOrigin says where a plugin candidate directory was found.
type pluginOrigin int

const (
	// pluginAtScanRoot is a scan root that holds a plugin.json.
	pluginAtScanRoot pluginOrigin = iota
	// pluginInGenericContainer is an entry of a scan root's plugins/
	// directory, a name other tools also use.
	pluginInGenericContainer
	// pluginInAgentsContainer is an entry of a scan root's
	// .agents/plugins/ directory.
	pluginInAgentsContainer
)

// pluginResult is the outcome of inspecting a plugin candidate.
type pluginResult int

const (
	// pluginAbsent means nothing was emitted: the directory has no
	// plugin.json, or its plugin.json was ignored.
	pluginAbsent pluginResult = iota
	// pluginRejected means a plugin resource with a non-OK status
	// was emitted and none of the plugin's components were loaded.
	pluginRejected
	// pluginValid means the plugin is StatusOK and owns its
	// components.
	pluginValid
	// pluginShadowed means the plugin is valid but another resource
	// already holds its Source, so nothing was emitted. Its directory
	// is still treated as a plugin root.
	pluginShadowed
)

// genericPluginContainerName is the plugins/ container directly under
// a scan root.
const genericPluginContainerName = "plugins"

// pluginContainerRelPaths are the directories, relative to a scan
// root, whose immediate subdirectories are plugin candidates.
var pluginContainerRelPaths = []string{
	genericPluginContainerName,
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

// canonicalExistingDir resolves symlinks in an existing directory
// path inside base. When the filesystem cannot report symlink targets
// (Windows substituted drives fail EvalSymlinks even for plain
// directories), the cleaned path is used only if it is a directory and
// no component from base down to dir is a link; otherwise the error
// stands.
func canonicalExistingDir(base, dir string) (string, error) {
	resolved, err := filepath.EvalSymlinks(dir)
	if err == nil {
		return resolved, nil
	}
	info, lerr := os.Lstat(dir)
	if lerr != nil || !info.IsDir() || hasLinkInside(base, dir) {
		return "", err
	}
	return filepath.Clean(dir), nil
}

// hasLinkInside reports whether any component from path up to and
// including base is a symlink or another reparse point such as a
// Windows junction, or cannot be read. Lstat follows links in all but
// the last component, so each one is checked. Components above base are
// trusted; for a path outside base only its last component is checked.
func hasLinkInside(base, path string) bool {
	base = filepath.Clean(base)
	for p := filepath.Clean(path); ; p = filepath.Dir(p) {
		info, err := os.Lstat(p)
		if err != nil || info.Mode()&(os.ModeSymlink|os.ModeIrregular) != 0 {
			return true
		}
		if p == base || !pathHasPrefix(p, base) {
			return false
		}
	}
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

// foreignManifestError reports a plugin.json JSON object whose $schema
// is absent or is not an Agent Plugins schema.
type foreignManifestError struct{ msg string }

func (e *foreignManifestError) Error() string { return e.msg }

// parsePluginManifest validates data as an Agent Plugins 1.0.0
// manifest. recognized is false when the document is not a JSON object
// or its $schema is absent or names a different format; err then says
// why, as a *foreignManifestError for the $schema cases. When
// recognized is true, a non-nil err rejects the plugin.
func parsePluginManifest(data []byte) (m pluginManifest, recognized bool, err error) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		if json.Valid(data) {
			return pluginManifest{}, false, xerrors.New("plugin.json must be a JSON object")
		}
		return pluginManifest{}, false, xerrors.Errorf("plugin.json is not valid JSON: %w", err)
	}
	if fields == nil {
		return pluginManifest{}, false, xerrors.New("plugin.json must be a JSON object")
	}

	var schema string
	raw, ok := fields["$schema"]
	if !ok {
		return pluginManifest{}, false, &foreignManifestError{msg: fmt.Sprintf("plugin.json has no $schema; Agent Plugins manifests declare %q", PluginSchemaV1)}
	}
	if json.Unmarshal(raw, &schema) != nil || !strings.HasPrefix(schema, pluginSchemaPrefix) {
		return pluginManifest{}, false, &foreignManifestError{msg: fmt.Sprintf("plugin.json $schema %s is not an Agent Plugins schema; Agent Plugins manifests declare %q", string(raw), PluginSchemaV1)}
	}
	if schema != PluginSchemaV1 {
		return pluginManifest{}, true, xerrors.Errorf("unsupported $schema %q; this agent supports only %q", schema, PluginSchemaV1)
	}

	rawName, ok := fields["name"]
	if !ok {
		return pluginManifest{}, true, xerrors.New("name is required")
	}
	if m.Name, ok = jsonString(rawName); !ok {
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
		value, ok := jsonString(raw)
		if !ok {
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
		var keywords []json.RawMessage
		if err := json.Unmarshal(raw, &keywords); err != nil || keywords == nil {
			return pluginManifest{}, true, xerrors.New("keywords must be an array of strings")
		}
		for _, keyword := range keywords {
			if _, ok := jsonString(keyword); !ok {
				return pluginManifest{}, true, xerrors.New("keywords must be an array of strings")
			}
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
			if _, ok := jsonString(value); !ok {
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
	for i, key := range unknown {
		if i == maxReportedUnknownFields {
			m.Warnings = append(m.Warnings, fmt.Sprintf("%d more unknown fields ignored", len(unknown)-i))
			break
		}
		m.Warnings = append(m.Warnings, fmt.Sprintf("unknown field %q ignored", key))
	}

	m.Version = strutil.Truncate(codersdk.SanitizePromptText(m.Version), MaxPluginVersionRunes)
	m.Description = strutil.Truncate(codersdk.SanitizePromptText(m.Description), MaxPluginDescriptionRunes)
	return m, true, nil
}

// jsonString decodes raw as a JSON string. JSON null is not a string.
func jsonString(raw json.RawMessage) (string, bool) {
	if strings.TrimSpace(string(raw)) == "null" {
		return "", false
	}
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		return "", false
	}
	return s, true
}

// pluginContainersFor returns the existing plugin-container
// directories under rootPath. A container that is a symlink, or whose
// resolved path leaves rootPath, is skipped.
func pluginContainersFor(rootPath string) []string {
	canonicalRoot, err := canonicalExistingDir(rootPath, rootPath)
	if err != nil {
		return nil
	}
	var out []string
	for _, rel := range pluginContainerRelPaths {
		container := filepath.Join(rootPath, rel)
		info, err := os.Lstat(container)
		if err != nil || !info.IsDir() {
			continue
		}
		resolved, err := canonicalExistingDir(rootPath, container)
		if err != nil || !pathHasPrefix(resolved, canonicalRoot) {
			continue
		}
		out = append(out, container)
	}
	return out
}

// discoverPlugin inspects dir as a plugin root. For a rejected plugin
// it emits one non-OK KindPlugin resource, unless an OK resource of
// another kind holds its Source, which then carries the error as a
// warning. For a valid plugin whose
// name is not yet in pluginNames it emits one KindPlugin resource and
// one KindSkill resource per skill under skills/. A valid plugin whose
// name is already held by the same canonical root emits nothing and
// is still reported as valid. A valid plugin whose Source an earlier
// valid resource already holds emits nothing, claims no name, and is
// reported as shadowed.
//
// Every path read inside the plugin is checked against the plugin
// root after symlink resolution. The root is canonicalized once so
// prefix checks, resource IDs, and Source values all use the same
// path.
//
// origin says where dir was found. At a scan root a plugin.json may
// belong to another tool, so it is ignored after a debug log unless it
// can be read and carries an Agent Plugins $schema. Another tool may
// also own a plugins/ entry, so a plugin.json there whose $schema is
// absent or not an Agent Plugins schema is ignored the same way. A
// plugins/ entry whose plugin.json cannot be read or is not a JSON
// object is reported as an invalid plugin even though its owner is
// unknown, as is every problem in .agents/plugins/.
//
// A plugin root whose path is not valid UTF-8 or is longer than
// workspacesdk.MaxContextSourceBytes is skipped with a warning, since
// coderd could not accept its Source.
func (r *Resolver) discoverPlugin(dir string, root ScanRoot, out *[]Resource, seenSource map[string]int, pluginNames map[string]string, origin pluginOrigin) pluginResult {
	pluginRoot, err := canonicalExistingDir(root.Path, dir)
	if err != nil {
		return pluginAbsent
	}
	manifestPath := filepath.Join(pluginRoot, pluginManifestFileName)
	info, err := os.Lstat(manifestPath)
	if err != nil {
		return pluginAbsent
	}
	if !utf8.ValidString(pluginRoot) {
		r.Logger.Warn(context.Background(), "skipping plugin whose path is not valid UTF-8; rename the directory to load it",
			slog.F("path", strings.ToValidUTF8(pluginRoot, "\uFFFD")))
		return pluginAbsent
	}
	if len(pluginRoot) > workspacesdk.MaxContextSourceBytes {
		r.Logger.Warn(context.Background(), "skipping plugin whose path is too long; move or rename the directory to load it",
			slog.F("path", pluginRoot), slog.F("max_bytes", workspacesdk.MaxContextSourceBytes))
		return pluginAbsent
	}
	logIgnored := func(reason string) {
		r.Logger.Debug(context.Background(), "ignoring plugin.json that is not an Agent Plugins manifest",
			slog.F("path", manifestPath), slog.F("reason", reason))
	}

	res := Resource{
		ID:         resourceID(KindPlugin, pluginRoot),
		Kind:       KindPlugin,
		Source:     pluginRoot,
		SizeBytes:  safeUint64(info.Size()),
		SourcePath: root.UserSource,
	}
	data, ok := r.readPluginManifest(manifestPath, info, pluginRoot, &res)
	if !ok {
		if origin == pluginAtScanRoot {
			logIgnored(res.Error)
			return pluginAbsent
		}
		r.appendResource(out, seenSource, res)
		return pluginRejected
	}

	manifest, recognized, err := parsePluginManifest(data)
	if !recognized {
		_, foreign := errors.AsType[*foreignManifestError](err)
		if origin == pluginAtScanRoot || (origin == pluginInGenericContainer && foreign) {
			logIgnored(err.Error())
			return pluginAbsent
		}
	}
	if err != nil {
		r.Logger.Debug(context.Background(), "invalid plugin manifest",
			slog.F("path", manifestPath), slog.Error(err))
		res.Status = StatusInvalid
		res.Error = err.Error()
		r.appendResource(out, seenSource, res)
		return pluginRejected
	}
	if claimedBy, taken := pluginNames[manifest.Name]; taken {
		if claimedBy == pluginRoot {
			return pluginValid
		}
		r.Logger.Debug(context.Background(), "duplicate plugin name",
			slog.F("path", manifestPath), slog.F("plugin", manifest.Name), slog.F("claimed_by", claimedBy))
		res.Status = StatusInvalid
		res.Error = fmt.Sprintf("plugin name %q is already used by %s; rename one of the plugins", manifest.Name, claimedBy)
		r.appendResource(out, seenSource, res)
		return pluginRejected
	}
	res.Name = manifest.Name
	res.PluginVersion = manifest.Version
	res.Description = manifest.Description
	warnings := manifest.Warnings

	skills, warn := r.pluginSkills(pluginRoot, root, manifest.Name)
	if warn != "" {
		warnings = append(warnings, warn)
	}

	res.Error = strings.Join(warnings, "; ")
	if !r.appendResource(out, seenSource, res) {
		return pluginShadowed
	}
	pluginNames[manifest.Name] = pluginRoot
	for _, skill := range skills {
		r.appendResource(out, seenSource, skill)
	}
	return pluginValid
}

// readPluginManifest reads the plugin.json at manifestPath, whose Lstat
// result is info, after checking that it resolves to a regular file
// inside pluginRoot and fits the per-resource cap. It records the size
// and content hash on res. On failure it also sets res.Status and
// res.Error and returns false.
func (r *Resolver) readPluginManifest(manifestPath string, info fs.FileInfo, pluginRoot string, res *Resource) ([]byte, bool) {
	readPath, readInfo, resolved, status, errMsg := resolveReadTarget(manifestPath, info, pluginRoot)
	if !resolved {
		res.Status = status
		res.Error = errMsg
		return nil, false
	}
	if !readInfo.Mode().IsRegular() {
		res.Status = StatusInvalid
		res.Error = "plugin.json is not a regular file"
		return nil, false
	}
	res.SizeBytes = safeUint64(readInfo.Size())
	if res.SizeBytes > r.MaxResourceBytes {
		res.Status = StatusOversize
		res.Error = fmt.Sprintf("file size %d exceeds per-resource cap of %d bytes", readInfo.Size(), r.MaxResourceBytes)
		if data, err := readFileCapped(readPath, safeInt64(r.MaxResourceBytes)); err == nil {
			res.ContentHash = sha256.Sum256(data)
		}
		return nil, false
	}
	data, err := os.ReadFile(readPath)
	if err != nil {
		res.Status = StatusUnreadable
		res.Error = err.Error()
		return nil, false
	}
	res.ContentHash = sha256.Sum256(data)
	return data, true
}

// pluginSkillDir is a skill directory candidate inside a plugin's
// skills/ container.
type pluginSkillDir struct {
	path string
	// escapeErr is set when path is a symlink whose target leaves the
	// plugin root.
	escapeErr string
}

// pluginSkillDirs lists the skill directory candidates in the skills/
// container under pluginDir. Following spec section 4.1, the container
// and each skill directory may be symlinks whose targets stay inside
// pluginRoot, the canonical form of pluginDir. A container that is
// missing yields no container and no warning; one that escapes
// pluginRoot or is not a directory yields no container and a warning.
// A skill directory symlink that escapes pluginRoot is returned with
// escapeErr set. A skill directory whose name is not valid UTF-8 or
// whose path is longer than workspacesdk.MaxContextSourceBytes is
// skipped and counted in warning.
func pluginSkillDirs(pluginDir, pluginRoot string) (container string, dirs []pluginSkillDir, warning string) {
	container = filepath.Join(pluginDir, pluginSkillsDirName)
	info, err := os.Lstat(container)
	if err != nil {
		return "", nil, ""
	}
	if info.Mode()&fs.ModeSymlink != 0 {
		target, err := filepath.EvalSymlinks(container)
		if err != nil {
			return "", nil, fmt.Sprintf("cannot resolve skills symlink: %v; skills not loaded", err)
		}
		if !pathHasPrefix(target, pluginRoot) {
			return "", nil, fmt.Sprintf("skills symlink target %q escapes plugin root; skills not loaded", target)
		}
		if info, err = os.Stat(target); err != nil {
			return "", nil, fmt.Sprintf("skills directory unreadable: %v; skills not loaded", err)
		}
	}
	if !info.IsDir() {
		return "", nil, "skills is not a directory; skills not loaded"
	}
	entries, err := os.ReadDir(container)
	if err != nil {
		return "", nil, fmt.Sprintf("skills directory unreadable: %v; skills not loaded", err)
	}
	invalidNames, longPaths := 0, 0
	add := func(d pluginSkillDir) {
		switch {
		case !utf8.ValidString(filepath.Base(d.path)):
			invalidNames++
		case len(d.path) > workspacesdk.MaxContextSourceBytes:
			longPaths++
		default:
			dirs = append(dirs, d)
		}
	}
	for _, e := range entries {
		dir := filepath.Join(container, e.Name())
		if e.Type()&fs.ModeSymlink == 0 {
			if e.IsDir() {
				add(pluginSkillDir{path: dir})
			}
			continue
		}
		target, err := filepath.EvalSymlinks(dir)
		if err != nil {
			continue
		}
		if st, err := os.Stat(target); err != nil || !st.IsDir() {
			continue
		}
		var escapeErr string
		if !pathHasPrefix(target, pluginRoot) {
			escapeErr = fmt.Sprintf("symlink target %q escapes plugin root %q", target, pluginRoot)
		}
		add(pluginSkillDir{path: dir, escapeErr: escapeErr})
	}
	var warnings []string
	if invalidNames > 0 {
		warnings = append(warnings, fmt.Sprintf("skill directories skipped because their names are not valid UTF-8: %d; rename them to load them", invalidNames))
	}
	if longPaths > 0 {
		warnings = append(warnings, fmt.Sprintf("skill directories skipped because their paths are longer than %d bytes: %d; shorten them to load them", workspacesdk.MaxContextSourceBytes, longPaths))
	}
	return container, dirs, strings.Join(warnings, "; ")
}

// pluginSkills returns one KindSkill resource per skill directory in
// pluginRoot's skills/ container that holds a SKILL.md, and the
// container warning from pluginSkillDirs. A skill directory or SKILL.md
// whose symlink target leaves pluginRoot is emitted as StatusInvalid.
func (r *Resolver) pluginSkills(pluginRoot string, root ScanRoot, pluginName string) ([]Resource, string) {
	_, dirs, warning := pluginSkillDirs(pluginRoot, pluginRoot)
	var skills []Resource
	for _, dir := range dirs {
		meta := filepath.Join(dir.path, skillMetaFileName)
		metaInfo, err := os.Lstat(meta)
		if err != nil {
			continue
		}
		if dir.escapeErr != "" {
			skills = append(skills, Resource{
				ID:         resourceID(KindSkill, dir.path),
				Kind:       KindSkill,
				Source:     dir.path,
				SizeBytes:  safeUint64(metaInfo.Size()),
				SourcePath: root.UserSource,
				Status:     StatusInvalid,
				Error:      dir.escapeErr,
				PluginName: pluginName,
			})
			continue
		}
		res, ok := r.readSkillMeta(pluginRoot, meta, metaInfo, root.UserSource)
		if !ok {
			continue
		}
		res.PluginName = pluginName
		skills = append(skills, res)
	}
	return skills, warning
}
