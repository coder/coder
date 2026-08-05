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
//
// A tokenizer failure mid-walk fails closed. json.Unmarshal skips values in
// ignored fields without fully materializing them (a huge exponent such as
// 1e999 errors here as a float64 overflow but unmarshals fine into a struct
// that drops the key), so treating a walk failure as acceptance would let a
// poisoned key stop the walk before a smuggled key is inspected.
func ValidateUnambiguous(properties map[string]any, input []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(input))
	// Numbers surface as json.Number so values that overflow float64
	// cannot abort the walk that Unmarshal would survive.
	decoder.UseNumber()
	token, err := decoder.Token()
	// Input with no top-level object has no object keys to smuggle, and
	// input that does not parse at all does not decode for the tool
	// either, so its own decode reports the failure.
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
			return walkError(err)
		}
		if token == json.Delim('}') {
			return nil
		}
		key, ok := token.(string)
		if !ok {
			return xerrors.Errorf("input holds a non-string object key %v", token)
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
			return walkError(err)
		}
		if err := validateValue(childSchema(properties, key), decoder, value, path); err != nil {
			return err
		}
	}
}

// walkError fails closed on a tokenizer error so the remainder of the
// object, which may hold a key the checks above would reject, is never
// skipped silently.
func walkError(err error) error {
	return xerrors.Errorf("input could not be fully inspected: %w", err)
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
				return walkError(err)
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
