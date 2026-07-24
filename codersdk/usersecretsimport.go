package codersdk

import (
	"encoding/json"
	"errors"
	"io"
	"strings"

	"golang.org/x/xerrors"
	"gopkg.in/yaml.v3"
)

// SecretsFileFormat identifies the on-disk format of a secrets file.
type SecretsFileFormat string

const (
	// SecretsFileFormatEnv is a dotenv-style file of KEY=VALUE lines.
	SecretsFileFormatEnv SecretsFileFormat = "env"
	// SecretsFileFormatJSON is a flat JSON object of string values.
	SecretsFileFormatJSON SecretsFileFormat = "json"
	// SecretsFileFormatYAML is a flat YAML mapping of string values.
	SecretsFileFormatYAML SecretsFileFormat = "yaml"
)

// MaxSecretsFileBytes bounds the raw size of a secrets file before parsing.
const MaxSecretsFileBytes = 1 << 20 // 1 MiB

type secretEntry struct {
	key   string
	value string
	line  int
}

// ParseSecretsFile parses a secrets file into CreateUserSecretRequests.
// It checks structure and duplicate keys; per-entry validation is left to
// ValidateCreateUserSecretRequest. EnvName is set only when the key passes
// env-name validation (best-effort; keys like MY-TOKEN or PATH get an empty
// EnvName so they are still imported without env injection).
func ParseSecretsFile(format SecretsFileFormat, content string) ([]CreateUserSecretRequest, error) {
	if len(content) > MaxSecretsFileBytes {
		return nil, xerrors.Errorf("secrets file exceeds the maximum allowed size of %d bytes", MaxSecretsFileBytes)
	}

	switch format {
	case SecretsFileFormatEnv, SecretsFileFormatJSON, SecretsFileFormatYAML:
	case "":
		return nil, xerrors.New("a secrets file format is required")
	default:
		return nil, xerrors.Errorf("unknown secrets file format %q", format)
	}

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

	if len(entries) == 0 {
		return nil, xerrors.New("no secrets found in file")
	}
	if len(entries) > MaxUserSecretsPerUserCount {
		return nil, xerrors.Errorf("secrets file contains %d secrets, which exceeds the maximum of %d secrets per user", len(entries), MaxUserSecretsPerUserCount)
	}

	if err := detectDuplicateKeys(entries); err != nil {
		return nil, err
	}

	reqs := make([]CreateUserSecretRequest, 0, len(entries))
	for _, e := range entries {
		req := CreateUserSecretRequest{Name: e.key, Value: e.value}
		// env_name uses a partial unique index (WHERE env_name != ''),
		// so multiple empty env_names are allowed.
		if UserSecretEnvNameValid(e.key) == nil {
			req.EnvName = e.key
		}
		reqs = append(reqs, req)
	}
	return reqs, nil
}

// Duplicate keys are rejected up front (citing the line for env files)
// instead of surfacing as a per-row uniqueness violation later.
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

// parseEnvSecrets parses a dotenv-style file into ordered entries. It supports a
// deliberately small subset of dotenv:
//   - KEY=VALUE lines, an optional "export " prefix, and full-line "#" comments.
//   - Single-quoted values are literal; double-quoted values support \n, \t,
//     \r, \\, and \" escapes and keep unknown escapes literal.
//
// It intentionally does NOT:
//   - expand $VAR or ${VAR}. Secrets frequently contain "$", and expansion would
//     silently corrupt them.
//   - strip inline comments, so PASS=abc#123 keeps the trailing #123.
//   - support multiline values. Use the JSON or YAML format for PEM keys or
//     certs.
//
// Duplicate keys are an error (see detectDuplicateKeys); a silent last-wins
// would drop a secret.
//
// This is hand-rolled rather than using joho/godotenv or hashicorp/go-envparse
// because those expand variables and/or strip inline comments (silent secret
// corruption) and return unordered maps, so we lose source order, line numbers,
// and duplicate detection. They would also add a dependency to the public
// codersdk package.
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

func parseEnvValue(rhs string, lineNum int) (string, error) {
	v := strings.TrimLeft(rhs, " \t")
	if v == "" {
		return "", nil
	}
	switch v[0] {
	case '"':
		inner, err := doubleQuotedInner(v, lineNum)
		if err != nil {
			return "", err
		}
		return unescapeDoubleQuoted(inner), nil
	case '\'':
		return singleQuotedInner(v, lineNum)
	default:
		return strings.TrimSpace(v), nil
	}
}

func doubleQuotedInner(v string, lineNum int) (string, error) {
	for i := 1; i < len(v); i++ {
		if v[i] != '"' || hasOddBackslashRun(v, i) {
			continue
		}
		if strings.Trim(v[i+1:], " \t") != "" {
			return "", xerrors.Errorf("line %d: unexpected data after closing double quote", lineNum)
		}
		return v[1:i], nil
	}
	return "", xerrors.Errorf("line %d: missing closing double quote", lineNum)
}

func hasOddBackslashRun(s string, before int) bool {
	count := 0
	for i := before - 1; i >= 0 && s[i] == '\\'; i-- {
		count++
	}
	return count%2 == 1
}

// singleQuotedInner returns the content between the opening single
// quote at index 0 and the first closing single quote. Single quotes
// have no escape sequences, so the first quote after the opener always
// closes the value. Only whitespace may follow the closing quote.
func singleQuotedInner(v string, lineNum int) (string, error) {
	for i := 1; i < len(v); i++ {
		if v[i] != '\'' {
			continue
		}
		if strings.Trim(v[i+1:], " \t") != "" {
			return "", xerrors.Errorf("line %d: unexpected data after closing single quote", lineNum)
		}
		return v[1:i], nil
	}
	return "", xerrors.Errorf("line %d: missing closing single quote", lineNum)
}

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

	if _, err := dec.Token(); err != nil {
		return nil, xerrors.Errorf("invalid JSON: %w", err)
	}
	var extra any
	if err := dec.Decode(&extra); errors.Is(err, io.EOF) {
		return entries, nil
	} else if err != nil {
		return nil, xerrors.Errorf("invalid JSON: %w", err)
	}
	return nil, xerrors.New("unexpected trailing data after JSON object")
}

func parseYAMLSecrets(content string) ([]secretEntry, error) {
	dec := yaml.NewDecoder(strings.NewReader(content))

	var root yaml.Node
	if err := dec.Decode(&root); err != nil {
		if errors.Is(err, io.EOF) {
			return nil, nil
		}
		return nil, xerrors.Errorf("invalid YAML: %w", err)
	}

	var extra yaml.Node
	if err := dec.Decode(&extra); err == nil {
		return nil, xerrors.New("YAML content must be a single document mapping secret names to string values")
	} else if !errors.Is(err, io.EOF) {
		return nil, xerrors.Errorf("invalid YAML: %w", err)
	}

	if root.Kind == 0 || len(root.Content) == 0 {
		return nil, nil
	}

	doc := root.Content[0]
	if doc.Kind != yaml.MappingNode {
		return nil, xerrors.New("YAML content must be a mapping of secret names to string values")
	}

	entries := make([]secretEntry, 0, len(doc.Content)/2)
	for i := 0; i+1 < len(doc.Content); i += 2 {
		keyNode := doc.Content[i]
		valNode := doc.Content[i+1]

		if keyNode.Kind != yaml.ScalarNode || (keyNode.Tag != "" && keyNode.Tag != "!!str") {
			return nil, xerrors.New("YAML keys must be strings")
		}
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
