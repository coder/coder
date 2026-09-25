// This file compiles screened schemas with the pinned gojsonschema v1.2.0
// and validates outputs with no network or filesystem access. The library
// resolves every uncached reference through the root loader's factory,
// whose built-in form performs http.Get or reads files, so the trusted
// Draft-07 meta-schema compiles in its own loader limited to the bundled
// copy, and each user schema compiles in a fresh loader that denies every
// load. Validation feedback is bounded because the library keeps every
// error, with descriptions that quote instance values.

package chatstructured

import (
	"cmp"
	"errors"
	"net/http"
	"slices"
	"strings"
	"sync"

	"github.com/xeipuuv/gojsonschema"
	"golang.org/x/xerrors"
)

var (
	// ErrValueMismatch is satisfied by every *ValidationError.
	ErrValueMismatch = xerrors.New("json value does not match the schema")
	errLoadDenied    = xerrors.New("json schema reference loading is denied")
)

const (
	// draft07MetaSchemaURL resolves to the library's bundled copy before any http.Get.
	draft07MetaSchemaURL = "http://json-schema.org/draft-07/schema"
	// patternPropertiesMaxNodes bounds outputs whose keys each recompile every pattern.
	patternPropertiesMaxNodes = 256
	maxIssues                 = 4
	maxIssuePathBytes         = 160
	maxValidationErrorBytes   = 1024
)

// Schema is a compiled structured output schema, safe for concurrent use.
type Schema struct {
	compiled          *gojsonschema.Schema
	patternProperties bool
}

// ValidationIssue locates one violation by path and rule name only.
type ValidationIssue struct {
	Path string // the library's field path, such as "(root)" or "a.0"
	Rule string // the library's error type, such as "invalid_type"
}

// ValidationError reports up to four issues of a mismatching value, the
// smallest by path and then rule, so the same input reports the same issues.
type ValidationError struct {
	Issues []ValidationIssue
}

// Error lists the issues within 1024 bytes of valid UTF-8.
func (e *ValidationError) Error() string {
	parts := make([]string, len(e.Issues))
	for i, issue := range e.Issues {
		parts[i] = issue.Path + " " + issue.Rule
	}
	return truncateUTF8(ErrValueMismatch.Error()+": "+strings.Join(parts, "; "), maxValidationErrorBytes)
}

// Unwrap makes errors.Is(err, ErrValueMismatch) hold.
func (*ValidationError) Unwrap() error { return ErrValueMismatch }

// restrictedFactory creates loaders that may load only allow, off the
// filesystem, and deny every other reference; the zero value denies all.
type restrictedFactory struct{ allow string }

func (f restrictedFactory) New(source string) gojsonschema.JSONLoader {
	if f.allow != "" && source == f.allow {
		return gojsonschema.NewReferenceLoaderFileSystem(source, deniedFS{})
	}
	return restrictedLoader{JSONLoader: gojsonschema.NewGoLoader(nil), deny: true}
}

// restrictedLoader replaces the embedded loader's factory, so a root loader
// resolves references through restrictedFactory.
type restrictedLoader struct {
	gojsonschema.JSONLoader
	factory restrictedFactory
	deny    bool
}

func (l restrictedLoader) LoaderFactory() gojsonschema.JSONLoaderFactory { return l.factory }

func (l restrictedLoader) LoadJSON() (any, error) {
	if l.deny {
		return nil, errLoadDenied
	}
	return l.JSONLoader.LoadJSON()
}

type deniedFS struct{}

func (deniedFS) Open(string) (http.File, error) { return nil, errLoadDenied }

// newSchemaLoader returns a fresh Draft-07 loader. Library meta-validation
// stays off: it compiles the meta-schema through the same loader's factory.
func newSchemaLoader() *gojsonschema.SchemaLoader {
	sl := gojsonschema.NewSchemaLoader()
	sl.Draft, sl.AutoDetect, sl.Validate = gojsonschema.Draft7, false, false
	return sl
}

// compileMetaSchema compiles the bundled Draft-07 meta-schema in a loader
// that can resolve nothing else.
func compileMetaSchema() (*gojsonschema.Schema, error) {
	f := restrictedFactory{allow: draft07MetaSchemaURL}
	return newSchemaLoader().Compile(restrictedLoader{JSONLoader: f.New(draft07MetaSchemaURL), factory: f})
}

var trustedMetaSchema = sync.OnceValues(compileMetaSchema)

// compileDenyAll compiles doc in a fresh loader that denies every
// reference load. Library errors can quote the schema, so they collapse to
// ErrInvalidSchema, joined with errLoadDenied when a load was attempted.
func compileDenyAll(doc map[string]any) (*gojsonschema.Schema, error) {
	s, err := newSchemaLoader().Compile(restrictedLoader{JSONLoader: gojsonschema.NewGoLoader(doc)})
	if errors.Is(err, errLoadDenied) {
		return nil, errors.Join(ErrInvalidSchema, errLoadDenied)
	}
	if err != nil {
		return nil, ErrInvalidSchema
	}
	return s, nil
}

// CompileSchema screens raw, meta-validates the original document as data
// against the trusted Draft-07 meta-schema, and compiles the sanitized copy
// offline. Rejections are the preflight's sentinels or ErrInvalidSchema.
func CompileSchema(raw []byte) (*Schema, error) {
	screened, err := preflightSchema(raw)
	if err != nil {
		return nil, err
	}
	meta, err := trustedMetaSchema()
	if err != nil {
		return nil, xerrors.Errorf("compile trusted meta-schema: %w", err)
	}
	result, err := meta.Validate(gojsonschema.NewGoLoader(screened.document))
	if err != nil || !result.Valid() {
		return nil, ErrInvalidSchema
	}
	compiled, err := compileDenyAll(screened.sanitized)
	if err != nil {
		return nil, err
	}
	return &Schema{compiled: compiled, patternProperties: screened.patternProperties}, nil
}

// Validate parses raw under the output caps, with at most 256 nodes when
// the schema uses patternProperties, and validates it. It returns the parsed
// value, the raw JSON sentinel of a cap, or a *ValidationError.
func (s *Schema) Validate(raw []byte) (any, error) {
	lim := outputValueLimits
	if s.patternProperties {
		lim.maxNodes = patternPropertiesMaxNodes
	}
	value, err := parseJSON(raw, lim)
	if err != nil {
		return nil, err
	}
	result, err := s.compiled.Validate(gojsonschema.NewBytesLoader(raw))
	if err != nil {
		return nil, xerrors.Errorf("validate structured output: %w", err)
	}
	if result.Valid() {
		return value, nil
	}
	// Keep the smallest issues in one pass over the library's full list.
	var issues []ValidationIssue
	for _, e := range result.Errors() {
		issue := ValidationIssue{Path: truncateUTF8(e.Field(), maxIssuePathBytes), Rule: e.Type()}
		i, _ := slices.BinarySearchFunc(issues, issue, func(a, b ValidationIssue) int {
			return cmp.Or(strings.Compare(a.Path, b.Path), strings.Compare(a.Rule, b.Rule))
		})
		if i < maxIssues {
			issues = slices.Insert(issues, i, issue)
			issues = issues[:min(len(issues), maxIssues)]
		}
	}
	return nil, &ValidationError{Issues: issues}
}

// truncateUTF8 cuts s to at most n bytes, dropping a partial final rune.
func truncateUTF8(s string, n int) string { return strings.ToValidUTF8(s[:min(len(s), n)], "") }
