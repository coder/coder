package codersdk

import (
	"encoding/json"
	"errors"
	"io"
	"strings"

	"golang.org/x/xerrors"
	"gopkg.in/yaml.v3"
)

// SecretsFileFormat identifies the on-disk format of an uploaded
// secrets file. It is shared by the HTTP import endpoint and is
// intended to be reused by a future `coder secret` CLI without change.
type SecretsFileFormat string

const (
	// SecretsFileFormatEnv is a dotenv-style file of KEY=VALUE lines.
	SecretsFileFormatEnv SecretsFileFormat = "env"
	// SecretsFileFormatJSON is a flat JSON object of string values.
	SecretsFileFormatJSON SecretsFileFormat = "json"
	// SecretsFileFormatYAML is a flat YAML mapping of string values.
	SecretsFileFormatYAML SecretsFileFormat = "yaml"
)

// MaxSecretsFileBytes bounds the raw size of an uploaded secrets file
// before parsing, guarding against resource-exhaustion inputs (huge
// files, deeply nested YAML, "billion laughs"). 1 MiB far exceeds the
// 200 KiB per-user value budget (MaxUserSecretsTotalValueBytes).
const MaxSecretsFileBytes = 1 << 20 // 1 MiB

// ImportUserSecretsRequest is the payload for the bulk secret import
// endpoint. Content is the raw file contents and Format selects the
// parser used to interpret it.
type ImportUserSecretsRequest struct {
	Format  SecretsFileFormat `json:"format"`
	Content string            `json:"content"`
}

// secretEntry is one parsed (key, value) pair in source order. line is
// 1-based and only meaningful for the env format (it is 0 for JSON and
// the key node's line for YAML); it is used to make duplicate-key and
// syntax errors point at the offending line.
type secretEntry struct {
	key   string
	value string
	line  int
}

// ParseSecretsFile parses an uploaded secrets file into
// CreateUserSecretRequests in source order. It does structural parsing
// and intra-file duplicate detection only; per-entry validation is left
// to ValidateCreateUserSecretRequest. Every format maps each KEY:VALUE
// to {Name: KEY, EnvName: KEY, Value: VALUE}, so one duplicate-KEY check
// covers duplicate names, env_names, and file_paths at once.
func ParseSecretsFile(format SecretsFileFormat, content string) ([]CreateUserSecretRequest, error) {
	// Reject oversized content before parsing so a malicious or
	// accidental huge upload cannot drive the parser at all.
	if len(content) > MaxSecretsFileBytes {
		return nil, xerrors.Errorf("secrets file exceeds the maximum allowed size of %d bytes", MaxSecretsFileBytes)
	}

	switch format {
	case SecretsFileFormatEnv, SecretsFileFormatJSON, SecretsFileFormatYAML:
		// Recognized format; fall through to parsing.
	case "":
		return nil, xerrors.New("a secrets file format is required")
	default:
		return nil, xerrors.Errorf("unknown secrets file format %q", format)
	}

	// Treat an empty or whitespace-only file uniformly across formats.
	if strings.TrimSpace(content) == "" {
		return nil, xerrors.New("no secrets found in file")
	}

	var (
		entries []secretEntry
		err     error
	)
	switch format {
	case SecretsFileFormatEnv:
		entries, err = parseEnvSecrets(content)
	case SecretsFileFormatJSON:
		entries, err = parseJSONSecrets(content)
	case SecretsFileFormatYAML:
		entries, err = parseYAMLSecrets(content)
	}
	if err != nil {
		return nil, err
	}

	// An env file of only comments, or an empty JSON/YAML object, parses
	// successfully but yields nothing to import.
	if len(entries) == 0 {
		return nil, xerrors.New("no secrets found in file")
	}

	if err := detectDuplicateKeys(entries); err != nil {
		return nil, err
	}

	reqs := make([]CreateUserSecretRequest, 0, len(entries))
	for _, e := range entries {
		reqs = append(reqs, CreateUserSecretRequest{
			Name:    e.key,
			EnvName: e.key,
			Value:   e.value,
		})
	}
	return reqs, nil
}

// detectDuplicateKeys scans for repeated keys in source order. Because
// the flat mapping sets Name == EnvName == KEY, catching duplicates
// here gives a clear up-front error (citing the line for env files)
// instead of a later per-row uniqueness violation.
func detectDuplicateKeys(entries []secretEntry) error {
	seen := make(map[string]struct{}, len(entries))
	for _, e := range entries {
		if _, ok := seen[e.key]; ok {
			if e.line > 0 {
				return xerrors.Errorf("duplicate key %q on line %d", e.key, e.line)
			}
			return xerrors.Errorf("duplicate key %q", e.key)
		}
		seen[e.key] = struct{}{}
	}
	return nil
}

// parseEnvSecrets parses dotenv-style content into ordered entries.
// CRLF is normalized to LF and a leading BOM stripped; lines are
// 1-based for errors. Blank lines and full-line '#' comments are
// skipped, and an optional leading "export " prefix is removed. Each
// line splits on the first '='; later '=' stay in the value. Values
// may be double-quoted (escapes \n \t \r \\ \" interpreted),
// single-quoted (literal), or unquoted (whitespace-trimmed). An inline
// '#' is kept literally rather than starting a comment, since silently
// truncating a secret value would be a footgun.
func parseEnvSecrets(content string) ([]secretEntry, error) {
	content = strings.ReplaceAll(content, "\r\n", "\n")
	content = strings.TrimPrefix(content, "\ufeff")

	var entries []secretEntry
	for i, raw := range strings.Split(content, "\n") {
		lineNum := i + 1

		if t := strings.TrimSpace(raw); t == "" || strings.HasPrefix(t, "#") {
			continue
		}

		work := stripExportPrefix(strings.TrimLeft(raw, " \t"))

		eq := strings.IndexByte(work, '=')
		if eq < 0 {
			return nil, xerrors.Errorf("line %d: expected KEY=VALUE but found no '='", lineNum)
		}

		key := strings.TrimSpace(work[:eq])
		if key == "" {
			return nil, xerrors.Errorf("line %d: missing key before '='", lineNum)
		}

		value, err := parseEnvValue(work[eq+1:], lineNum)
		if err != nil {
			return nil, err
		}
		entries = append(entries, secretEntry{key: key, value: value, line: lineNum})
	}
	return entries, nil
}

// stripExportPrefix removes a leading "export " (the word export
// followed by whitespace). A line like "export=foo" is left untouched
// so the key becomes "export".
func stripExportPrefix(s string) string {
	const kw = "export"
	if !strings.HasPrefix(s, kw) {
		return s
	}
	rest := s[len(kw):]
	if rest == "" || (rest[0] != ' ' && rest[0] != '\t') {
		return s
	}
	return strings.TrimLeft(rest, " \t")
}

// parseEnvValue interprets the right-hand side of an env assignment.
func parseEnvValue(rhs string, lineNum int) (string, error) {
	v := strings.TrimLeft(rhs, " \t")
	if v == "" {
		return "", nil
	}
	switch v[0] {
	case '"':
		// Double-quoted: runs to the matching closing quote, with the
		// permitted escape sequences interpreted.
		inner, ok := quotedInner(v, '"')
		if !ok {
			return "", xerrors.Errorf("line %d: missing closing double quote", lineNum)
		}
		return unescapeDoubleQuoted(inner), nil
	case '\'':
		// Single-quoted: verbatim, no escape processing.
		inner, ok := quotedInner(v, '\'')
		if !ok {
			return "", xerrors.Errorf("line %d: missing closing single quote", lineNum)
		}
		return inner, nil
	default:
		// Unquoted: trim surrounding whitespace, keep '#' literally.
		return strings.TrimSpace(v), nil
	}
}

// quotedInner returns the content between the opening quote (v[0]) and
// the matching closing quote, which must be the last character after
// right-trimming whitespace. ok is false when no closing quote is found.
func quotedInner(v string, quote byte) (string, bool) {
	trimmed := strings.TrimRight(v, " \t")
	if len(trimmed) < 2 || trimmed[len(trimmed)-1] != quote {
		return "", false
	}
	return trimmed[1 : len(trimmed)-1], true
}

// unescapeDoubleQuoted interprets the escapes permitted inside a
// double-quoted env value: \n \t \r \\ \". Any other backslash
// sequence, or a trailing backslash, is preserved literally.
func unescapeDoubleQuoted(s string) string {
	if !strings.Contains(s, "\\") {
		return s
	}
	buf := make([]byte, 0, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c != '\\' || i == len(s)-1 {
			buf = append(buf, c)
			continue
		}
		switch next := s[i+1]; next {
		case 'n':
			buf = append(buf, '\n')
		case 't':
			buf = append(buf, '\t')
		case 'r':
			buf = append(buf, '\r')
		case '\\':
			buf = append(buf, '\\')
		case '"':
			buf = append(buf, '"')
		default:
			buf = append(buf, '\\', next)
		}
		i++
	}
	return string(buf)
}

// parseJSONSecrets parses a flat JSON object of string values into
// ordered entries using a token decoder, so source order is preserved,
// duplicate keys remain observable, and non-string or nested values are
// rejected.
func parseJSONSecrets(content string) ([]secretEntry, error) {
	dec := json.NewDecoder(strings.NewReader(content))

	tok, err := dec.Token()
	if err != nil {
		return nil, xerrors.Errorf("invalid JSON: %w", err)
	}
	if delim, ok := tok.(json.Delim); !ok || delim != '{' {
		return nil, xerrors.New("JSON content must be an object mapping secret names to string values")
	}

	var entries []secretEntry
	for dec.More() {
		keyTok, err := dec.Token()
		if err != nil {
			return nil, xerrors.Errorf("invalid JSON: %w", err)
		}
		key, ok := keyTok.(string)
		if !ok {
			return nil, xerrors.New("invalid JSON object key")
		}

		valTok, err := dec.Token()
		if err != nil {
			return nil, xerrors.Errorf("invalid JSON: %w", err)
		}
		switch val := valTok.(type) {
		case string:
			entries = append(entries, secretEntry{key: key, value: val})
		case json.Delim:
			return nil, xerrors.Errorf("value for key %q must be a string, not a nested object or array", key)
		default:
			return nil, xerrors.Errorf("value for key %q must be a string", key)
		}
	}

	// Consume the closing brace, then ensure nothing follows the
	// top-level object.
	if _, err := dec.Token(); err != nil {
		return nil, xerrors.Errorf("invalid JSON: %w", err)
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return nil, xerrors.New("unexpected trailing data after JSON object")
	}

	return entries, nil
}

// parseYAMLSecrets parses a flat YAML mapping of string values into
// ordered entries. The top level must be a mapping with scalar string
// values; non-string scalars, nested nodes, and multi-document streams
// are rejected so no value is silently coerced or dropped. Duplicate
// keys are caught by the shared duplicate check.
func parseYAMLSecrets(content string) ([]secretEntry, error) {
	dec := yaml.NewDecoder(strings.NewReader(content))

	var root yaml.Node
	if err := dec.Decode(&root); err != nil {
		// An empty document or comments-only file decodes to nothing.
		if errors.Is(err, io.EOF) {
			return nil, nil
		}
		return nil, xerrors.Errorf("invalid YAML: %w", err)
	}

	// Reject additional documents so a multi-document stream cannot
	// silently drop secrets. A bare trailing "---" or comments-only
	// tail decodes to a null document and is allowed.
	for {
		var extra yaml.Node
		err := dec.Decode(&extra)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, xerrors.Errorf("invalid YAML: %w", err)
		}
		if yamlDocumentHasContent(extra) {
			return nil, xerrors.New("YAML content must be a single document mapping secret names to string values")
		}
	}

	// An empty document or comments-only file decodes to a zero node.
	if root.Kind == 0 || len(root.Content) == 0 {
		return nil, nil
	}

	doc := root.Content[0]
	if doc.Kind != yaml.MappingNode {
		return nil, xerrors.New("YAML content must be a mapping of secret names to string values")
	}

	entries := make([]secretEntry, 0, len(doc.Content)/2)
	// Mapping node content alternates key, value, key, value, ...
	for i := 0; i+1 < len(doc.Content); i += 2 {
		keyNode := doc.Content[i]
		valNode := doc.Content[i+1]

		if valNode.Kind != yaml.ScalarNode {
			return nil, xerrors.Errorf("value for key %q must be a string, not a nested mapping or sequence", keyNode.Value)
		}
		if valNode.Tag != "" && valNode.Tag != "!!str" {
			return nil, xerrors.Errorf("value for key %q must be a string (quote the value if it is numeric or boolean)", keyNode.Value)
		}
		entries = append(entries, secretEntry{key: keyNode.Value, value: valNode.Value, line: keyNode.Line})
	}
	return entries, nil
}

// yamlDocumentHasContent reports whether a decoded YAML document node
// carries data. A bare trailing "---" or comments-only tail decodes to
// a null scalar (no content); any other node is a real second document.
func yamlDocumentHasContent(doc yaml.Node) bool {
	if doc.Kind == 0 || len(doc.Content) == 0 {
		return false
	}
	child := doc.Content[0]
	if child.Kind == yaml.ScalarNode && child.Tag == "!!null" {
		return false
	}
	return true
}
