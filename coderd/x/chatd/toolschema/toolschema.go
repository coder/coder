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
	return validateObject(properties, decoder, "")
}

func validateObject(properties map[string]any, decoder *json.Decoder, parent string) error {
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
		if err := checkKeyCase(properties, key, path); err != nil {
			return err
		}
		value, err := decoder.Token()
		if err != nil {
			return nil
		}
		if err := validateValue(childSchema(properties, key), decoder, value, path); err != nil {
			return err
		}
	}
}

func validateValue(schema map[string]any, decoder *json.Decoder, token json.Token, path string) error {
	switch token {
	case json.Delim('{'):
		properties, _ := schema["properties"].(map[string]any)
		return validateObject(properties, decoder, path)
	case json.Delim('['):
		items, _ := schema["items"].(map[string]any)
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

func checkKeyCase(properties map[string]any, key, path string) error {
	if _, exact := properties[key]; exact {
		return nil
	}
	for _, name := range slices.Sorted(maps.Keys(properties)) {
		if strings.EqualFold(name, key) {
			return xerrors.Errorf("input key %q differs from schema property %q only by case", path, name)
		}
	}
	return nil
}

func childSchema(properties map[string]any, key string) map[string]any {
	name := key
	if _, freeForm := properties[freeFormPropertyName]; freeForm {
		name = freeFormPropertyName
	}
	schema, _ := properties[name].(map[string]any)
	return schema
}
