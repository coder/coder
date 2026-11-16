package agentcontext

import (
	"bytes"
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
	pluginManifestFileName = "plugin.json"
	// pluginSchemaPrefix identifies a plugin.json that belongs to the
	// Agent Plugins format rather than another tool.
	pluginSchemaPrefix = "https://agent-plugins.org/schemas/"
	// PluginSchemaV1 is the only $schema value the resolver accepts.
	PluginSchemaV1 = pluginSchemaPrefix + "1.0.0/plugin.schema.json"

	// The spec sets no length limit on version or description
	// (section 5.4); these caps are Coder's own.
	maxPluginVersionRunes     = 64
	maxPluginDescriptionRunes = 1024
)

// maxReportedUnknownFields bounds how many unknown manifest keys are
// named in a plugin's warnings.
const maxReportedUnknownFields = 10

var utf8BOM = []byte("\xef\xbb\xbf")

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
	// pluginRejected means the plugin is not valid and none of its
	// components were loaded. It is reported as a non-OK plugin
	// resource, or as a warning on an OK resource that already holds
	// the same Source.
	pluginRejected
	// pluginValid means the plugin is valid. Its components are
	// loaded unless another resource already holds its Source.
	pluginValid
)

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
		if bytes.HasPrefix(data, utf8BOM) {
			return pluginManifest{}, false, xerrors.New("plugin.json starts with a UTF-8 byte order mark; save it without one")
		}
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
	if json.Unmarshal(raw, &schema) != nil {
		return pluginManifest{}, false, &foreignManifestError{msg: fmt.Sprintf("plugin.json $schema is not a string; Agent Plugins manifests declare %q", PluginSchemaV1)}
	}
	if !strings.HasPrefix(schema, pluginSchemaPrefix) {
		return pluginManifest{}, false, &foreignManifestError{msg: fmt.Sprintf("plugin.json $schema %q is not an Agent Plugins schema; Agent Plugins manifests declare %q", schema, PluginSchemaV1)}
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

	// Extension namespace values are not validated: spec section 8.1
	// ignores namespaces a client does not implement.
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

	m.Version = pluginText(m.Version, maxPluginVersionRunes)
	m.Description = pluginText(m.Description, maxPluginDescriptionRunes)
	return m, true, nil
}

// pluginText sanitizes and caps a manifest string carried to coderd. It
// also removes U+0000, which coderd's jsonb storage cannot hold.
func pluginText(s string, maxRunes int) string {
	return strutil.Truncate(codersdk.SanitizePromptText(strings.ReplaceAll(s, "\x00", "")), maxRunes)
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

// discoverPlugin inspects dir as a plugin root. A rejected plugin is
// reported as one non-OK KindPlugin resource, or as a warning on an OK
// resource that already holds its Source. A valid plugin emits a
// KindPlugin resource and claims its name in pluginNames. A valid
// plugin whose name the same canonical root already holds emits
// nothing, as does one whose Source is already held.
//
// The root is canonicalized once and used for prefix checks, IDs, and
// Source. Every path read inside the plugin must resolve within it.
//
// Another tool may own a plugin.json at a scan root or in plugins/.
// At a scan root, one that is unreadable or lacks an Agent Plugins
// $schema is ignored. In plugins/, only an absent or foreign $schema
// is ignored. In .agents/plugins/, every problem with a plugin.json is
// reported, including a plugin.json that cannot be stat'ed for a
// reason other than not existing.
//
// A root whose path is not valid UTF-8 or exceeds
// workspacesdk.MaxContextSourceBytes is skipped, since coderd rejects
// such a Source.
func (r *Resolver) discoverPlugin(dir string, root ScanRoot, out *[]Resource, index *resourceIndex, pluginNames map[string]string, origin pluginOrigin) pluginResult {
	pluginRoot, err := canonicalExistingDir(root.Path, dir)
	if err != nil {
		return pluginAbsent
	}
	manifestPath := filepath.Join(pluginRoot, pluginManifestFileName)
	info, statErr := os.Lstat(manifestPath)
	if statErr != nil && (origin != pluginInAgentsContainer || errors.Is(statErr, fs.ErrNotExist)) {
		return pluginAbsent
	}
	if !r.pluginSourceUsable(pluginRoot) {
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
		SourcePath: root.UserSource,
	}
	if statErr != nil {
		res.Status = StatusUnreadable
		res.Error = statErr.Error()
		r.appendResource(out, index, res)
		return pluginRejected
	}
	res.SizeBytes = safeUint64(info.Size())
	data, ok := r.readPluginManifest(manifestPath, info, pluginRoot, &res)
	if !ok {
		if origin == pluginAtScanRoot {
			logIgnored(res.Error)
			return pluginAbsent
		}
		r.appendResource(out, index, res)
		return pluginRejected
	}

	manifest, recognized, err := parsePluginManifest(data)
	if !recognized {
		foreign := errors.As(err, new(*foreignManifestError))
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
		r.appendResource(out, index, res)
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
		r.appendResource(out, index, res)
		return pluginRejected
	}
	res.Name = manifest.Name
	res.PluginVersion = manifest.Version
	res.Description = manifest.Description
	res.Error = strings.Join(manifest.Warnings, "; ")
	if !r.appendResource(out, index, res) {
		return pluginValid
	}
	pluginNames[manifest.Name] = pluginRoot
	return pluginValid
}

// pluginSourceUsable reports whether path can be sent as a plugin
// Source: coderd rejects one that is not valid UTF-8 or exceeds
// workspacesdk.MaxContextSourceBytes. A rejected path is logged.
func (r *Resolver) pluginSourceUsable(path string) bool {
	if !utf8.ValidString(path) {
		r.Logger.Warn(context.Background(), "skipping plugin whose path is not valid UTF-8; rename the directory to load it",
			slog.F("path", strings.ToValidUTF8(path, "\uFFFD")))
		return false
	}
	if len(path) > workspacesdk.MaxContextSourceBytes {
		r.Logger.Warn(context.Background(), "skipping plugin whose path is too long; move or rename the directory to load it",
			slog.F("path", path), slog.F("max_bytes", workspacesdk.MaxContextSourceBytes))
		return false
	}
	return true
}

// reportSymlinkedPluginDirs emits a StatusInvalid KindPlugin resource
// for each symlink in container that leads to a directory holding a
// plugin.json. Symlinked plugin directories are not loaded. The
// resource Source is the link path inside the canonical container.
func (r *Resolver) reportSymlinkedPluginDirs(container string, root ScanRoot, out *[]Resource, index *resourceIndex) {
	canonicalContainer, err := canonicalExistingDir(root.Path, container)
	if err != nil {
		return
	}
	entries, err := os.ReadDir(canonicalContainer)
	if err != nil {
		return
	}
	for _, e := range entries {
		if e.Type()&fs.ModeSymlink == 0 {
			continue
		}
		link := filepath.Join(canonicalContainer, e.Name())
		if _, err := os.Lstat(filepath.Join(link, pluginManifestFileName)); err != nil {
			continue
		}
		if !r.pluginSourceUsable(link) {
			continue
		}
		r.appendResource(out, index, Resource{
			ID:         resourceID(KindPlugin, link),
			Kind:       KindPlugin,
			Source:     link,
			SourcePath: root.UserSource,
			Status:     StatusInvalid,
			Error:      "plugin directory is a symlink, and symlinked plugin directories are not loaded; move the plugin here to load it",
		})
	}
}

// readPluginManifest reads manifestPath after checking that it
// resolves to a regular file inside pluginRoot and fits the
// per-resource cap. It records size and content hash on res; on
// failure it also sets res.Status and res.Error and returns false.
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
