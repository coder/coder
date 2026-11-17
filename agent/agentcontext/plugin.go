package agentcontext

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"reflect"
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

	if _, ok := fields["name"]; !ok {
		return pluginManifest{}, true, xerrors.New("name is required")
	}
	decoded, err := decodePluginManifest(fields)
	if err != nil {
		return pluginManifest{}, true, err
	}
	if err := workspacesdk.ValidatePluginName(string(decoded.Name)); err != nil {
		return pluginManifest{}, true, err
	}
	m.Name = string(decoded.Name)
	m.Version = string(decoded.Version)
	m.Description = string(decoded.Description)

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

// pluginManifestJSON holds the typed fields of a 1.0.0 manifest.
// $schema, extensions, and unknown keys are checked on the raw object.
type pluginManifestJSON struct {
	Name        manifestString    `json:"name"`
	Version     manifestString    `json:"version"`
	Description manifestString    `json:"description"`
	Homepage    manifestString    `json:"homepage"`
	Repository  manifestString    `json:"repository"`
	License     manifestString    `json:"license"`
	Keywords    *[]manifestString `json:"keywords"`
	Author      *struct {
		Name  manifestString `json:"name"`
		Email manifestString `json:"email"`
		URL   manifestString `json:"url"`
	} `json:"author"`
}

// manifestString is a manifest string field. Unlike string, it rejects
// JSON null, which the schema does not allow.
type manifestString string

func (s *manifestString) UnmarshalJSON(data []byte) error {
	if string(data) == "null" {
		return &json.UnmarshalTypeError{Value: "null", Type: reflect.TypeFor[string]()}
	}
	return json.Unmarshal(data, (*string)(s))
}

// decodePluginManifest decodes the schema-defined keys of fields and
// rejects values of the wrong type and unknown author keys.
func decodePluginManifest(fields map[string]json.RawMessage) (pluginManifestJSON, error) {
	// encoding/json matches keys case-insensitively, so only the exact
	// schema keys are decoded.
	known := make(map[string]json.RawMessage, len(pluginManifestFields))
	for key, value := range fields {
		if _, ok := pluginManifestFields[key]; ok {
			known[key] = value
		}
	}
	data, err := json.Marshal(known)
	if err != nil {
		return pluginManifestJSON{}, xerrors.Errorf("encode manifest fields: %w", err)
	}
	var m pluginManifestJSON
	if err := json.Unmarshal(data, &m); err != nil {
		var typeErr *json.UnmarshalTypeError
		if !errors.As(err, &typeErr) {
			return pluginManifestJSON{}, err
		}
		return pluginManifestJSON{}, manifestTypeError(typeErr.Field)
	}
	// A null keywords or author leaves its pointer nil.
	if _, ok := fields["keywords"]; ok && m.Keywords == nil {
		return pluginManifestJSON{}, manifestTypeError("keywords")
	}
	if _, ok := fields["author"]; ok && m.Author == nil {
		return pluginManifestJSON{}, manifestTypeError("author")
	}
	if raw, ok := fields["author"]; ok {
		var author map[string]json.RawMessage
		if err := json.Unmarshal(raw, &author); err != nil {
			return pluginManifestJSON{}, manifestTypeError("author")
		}
		for _, key := range slices.Sorted(maps.Keys(author)) {
			if _, known := pluginAuthorFields[key]; !known {
				return pluginManifestJSON{}, xerrors.Errorf("author contains unknown field %q", key)
			}
		}
	}
	return m, nil
}

// manifestTypeError describes a manifest field whose value has the
// wrong JSON type. field is a dotted path such as "author.name".
func manifestTypeError(field string) error {
	switch field {
	case "keywords":
		return xerrors.New("keywords must be an array of strings")
	case "author":
		return xerrors.New("author must be an object")
	}
	return xerrors.Errorf("%s must be a string", field)
}

// pluginText sanitizes and caps a manifest string carried to coderd.
func pluginText(s string, maxRunes int) string {
	return strutil.Truncate(codersdk.SanitizePromptText(withoutNUL(s)), maxRunes)
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
	file, err := r.readPluginManifest(manifestPath, info, pluginRoot)
	res.SizeBytes = file.size
	res.ContentHash = file.hash
	if err != nil {
		if origin == pluginAtScanRoot {
			logIgnored(err.Error())
			return pluginAbsent
		}
		res.Status = errorStatus(err)
		res.Error = err.Error()
		r.appendResource(out, index, res)
		return pluginRejected
	}

	manifest, recognized, err := parsePluginManifest(file.data)
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

// pluginManifestFile is a plugin.json as read by readPluginManifest.
type pluginManifestFile struct {
	data []byte
	size uint64
	// hash covers data, or the first MaxResourceBytes of an oversize
	// file. It is zero when the file could not be read.
	hash [32]byte
}

// resourceError is a failure to load a resource, reported with status.
type resourceError struct {
	status ResourceStatus
	msg    string
}

func (e *resourceError) Error() string { return e.msg }

// errorStatus returns the status a *resourceError in err carries, or
// StatusInvalid.
func errorStatus(err error) ResourceStatus {
	var resErr *resourceError
	if errors.As(err, &resErr) {
		return resErr.status
	}
	return StatusInvalid
}

// readPluginManifest reads manifestPath after checking that it
// resolves to a regular file inside pluginRoot and fits the
// per-resource cap. The returned file carries the size and, when the
// file could be read, the content hash even if err is non-nil; err is
// a *resourceError that errorStatus maps to the resource status.
func (r *Resolver) readPluginManifest(manifestPath string, info fs.FileInfo, pluginRoot string) (pluginManifestFile, error) {
	file := pluginManifestFile{size: safeUint64(info.Size())}
	readPath, readInfo, resolved, status, errMsg := resolveReadTarget(manifestPath, info, pluginRoot)
	if !resolved {
		return file, &resourceError{status: status, msg: errMsg}
	}
	if !readInfo.Mode().IsRegular() {
		return file, &resourceError{status: StatusInvalid, msg: "plugin.json is not a regular file"}
	}
	file.size = safeUint64(readInfo.Size())
	if file.size > r.MaxResourceBytes {
		if data, err := readFileCapped(readPath, safeInt64(r.MaxResourceBytes)); err == nil {
			file.hash = sha256.Sum256(data)
		}
		return file, &resourceError{
			status: StatusOversize,
			msg:    fmt.Sprintf("file size %d exceeds per-resource cap of %d bytes", readInfo.Size(), r.MaxResourceBytes),
		}
	}
	data, err := os.ReadFile(readPath)
	if err != nil {
		return file, &resourceError{status: StatusUnreadable, msg: err.Error()}
	}
	file.data = data
	file.hash = sha256.Sum256(data)
	return file, nil
}
