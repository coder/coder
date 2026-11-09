// Package toolschema rejects tool inputs whose object keys the Go decoder
// and a case-sensitive reader resolve differently.
package toolschema

import (
	"bytes"
	"encoding/json"
	"maps"
	"slices"
	"strings"

	"golang.org/x/xerrors"
)

// freeFormPropertyName is the property name fantasy generates for
// map[string]T inputs. Its keys are data, so they are checked against the
// value schema behind this name rather than against a fixed property set.
const freeFormPropertyName = "*"

// ValidateUnambiguous reports an error when input holds an object key that
// encoding/json folds into a declared property but a case-sensitive reader
// treats as distinct, or when one object repeats a key. Either lets code
// inspecting the raw input read one value while the tool executes another.
//
// properties is a fantasy ToolInfo.Parameters map, keyed by property name.
// Declared properties are collected across anyOf, oneOf, and allOf branches;
// this checks key ambiguity without validating which branches match.
// Keys matching no property are ignored because a generated struct decoder
// drops them. A tool with a hand-written decoder that reads undeclared keys
// has to reject ambiguous spellings of those keys itself.
func ValidateUnambiguous(properties map[string]any, input []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(input))
	token, err := decoder.Token()
	// Input that does not parse here does not decode for the tool either,
	// so its own decode reports the failure.
	if err != nil || token != json.Delim('{') {
		return nil
	}
	return validateObject([]map[string]any{properties}, decoder, "")
}

func validateObject(propertySets []map[string]any, decoder *json.Decoder, parent string) error {
	seen := make(map[string]struct{})
	for {
		token, err := decoder.Token()
		if err != nil {
			return nil
		}
		if token == json.Delim('}') {
			return nil
		}
		key, ok := token.(string)
		if !ok {
			return nil
		}
		path := key
		if parent != "" {
			path = parent + "." + key
		}
		if _, duplicate := seen[key]; duplicate {
			return xerrors.Errorf("input repeats the key %q", path)
		}
		seen[key] = struct{}{}
		if err := checkKeyCase(propertySets, key, path); err != nil {
			return err
		}
		value, err := decoder.Token()
		if err != nil {
			return nil
		}
		if err := validateValue(childSchemas(propertySets, key), decoder, value, path); err != nil {
			return err
		}
	}
}

func validateValue(schemas []map[string]any, decoder *json.Decoder, token json.Token, path string) error {
	switch token {
	case json.Delim('{'):
		var propertySets []map[string]any
		for _, schema := range schemas {
			propertySets = append(propertySets, schemaChildren(schema, "properties")...)
		}
		return validateObject(propertySets, decoder, path)
	case json.Delim('['):
		var items []map[string]any
		for _, schema := range schemas {
			items = append(items, schemaChildren(schema, "items")...)
		}
		for {
			next, err := decoder.Token()
			if err != nil {
				return nil
			}
			if next == json.Delim(']') {
				return nil
			}
			if err := validateValue(items, decoder, next, path+"[]"); err != nil {
				return err
			}
		}
	}
	return nil
}

// schemaChildren collects a keyword's schemas from every composition branch.
// Keeping them separate preserves nested schemas for shared property names.
func schemaChildren(schema map[string]any, keyword string) []map[string]any {
	var children []map[string]any
	if child, ok := schema[keyword].(map[string]any); ok {
		children = append(children, child)
	}
	for _, composition := range []string{"anyOf", "oneOf", "allOf"} {
		branches, _ := schema[composition].([]any)
		for _, branch := range branches {
			if branch, ok := branch.(map[string]any); ok {
				children = append(children, schemaChildren(branch, keyword)...)
			}
		}
	}
	return children
}

func checkKeyCase(propertySets []map[string]any, key, path string) error {
	for _, properties := range propertySets {
		for _, name := range slices.Sorted(maps.Keys(properties)) {
			if name != key && strings.EqualFold(name, key) {
				return xerrors.Errorf("input key %q differs from schema property %q only by case", path, name)
			}
		}
	}
	return nil
}

func childSchemas(propertySets []map[string]any, key string) []map[string]any {
	var schemas []map[string]any
	for _, properties := range propertySets {
		name := key
		if _, freeForm := properties[freeFormPropertyName]; freeForm {
			name = freeFormPropertyName
		}
		if schema, ok := properties[name].(map[string]any); ok {
			schemas = append(schemas, schema)
		}
	}
	return schemas
}
