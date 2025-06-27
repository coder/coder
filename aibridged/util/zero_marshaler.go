package util

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"

	"github.com/jmespath/go-jmespath"
)

var ErrEmptyPath = errors.New("empty path provided")

// MarshalNoZero serializes v to JSON, omitting any struct field whose entire
// value graph is zero **unless** the field carries a `no_omit` tag or matches
// one of the provided JMESPath exclusion expressions.
func MarshalNoZero(v any, exclusions ...string) ([]byte, error) {
	cleaned, zero := prune(reflect.ValueOf(v), false, exclusions, "")
	if zero {
		return []byte("null"), nil
	}
	return json.Marshal(cleaned)
}

// matchesExclusion checks if the current path matches any of the JMESPath exclusion patterns.
func matchesExclusion(path string, exclusions []string) bool {
	if len(exclusions) == 0 || path == "" {
		return false
	}

	// Create a temporary object with the path structure to test against
	// For example, if path is "user.profile.name", we create {"user":{"profile":{"name":true}}}
	testObj, err := createTestObject(path)
	if err != nil {
		return false
	}

	for _, exclusion := range exclusions {
		result, err := jmespath.Search(exclusion, testObj)
		if err != nil {
			continue
		}
		if result != nil {
			return true
		}
	}
	return false
}

// createTestObject creates a nested object structure from a dot-notation path
// for testing against JMESPath expressions.
func createTestObject(path string) (map[string]any, error) {
	if path == "" {
		return nil, ErrEmptyPath
	}

	parts := strings.Split(path, ".")
	result := make(map[string]any)
	current := result

	for i, part := range parts {
		// Handle array notation like "items[0]"
		if strings.Contains(part, "[") && strings.Contains(part, "]") {
			// Extract array name and index
			arrayName := part[:strings.Index(part, "[")]
			indexStr := part[strings.Index(part, "[")+1 : strings.Index(part, "]")]

			// Create array structure
			var arr []any
			if idx := parseArrayIndex(indexStr); idx >= 0 {
				// Create array with enough elements
				for j := 0; j <= idx; j++ {
					arr = append(arr, make(map[string]any))
				}
				current[arrayName] = arr
				if i == len(parts)-1 {
					arr[idx] = true // Mark as matched for final element
				} else {
					if nextMap, ok := arr[idx].(map[string]any); ok {
						current = nextMap
					} else {
						return nil, fmt.Errorf("invalid path structure at %s", part)
					}
				}
			}
		} else {
			if i == len(parts)-1 {
				current[part] = true // Mark final element as matched
			} else {
				current[part] = make(map[string]any)
				if nextMap, ok := current[part].(map[string]any); ok {
					current = nextMap
				} else {
					return nil, fmt.Errorf("invalid path structure at %s", part)
				}
			}
		}
	}

	return result, nil
}

// parseArrayIndex extracts numeric index from array notation, returns -1 if invalid.
func parseArrayIndex(indexStr string) int {
	// Simple numeric parsing - extend if needed for more complex patterns
	if indexStr == "*" {
		return 0 // Treat wildcard as index 0 for testing
	}

	var idx int
	if _, err := fmt.Sscanf(indexStr, "%d", &idx); err == nil {
		return idx
	}
	return -1
}

// prune walks the value tree and returns (cleanedValue, isZeroTree).
// `forceKeep` is true when an ancestor field has the `no_omit` tag or matches an exclusion.
// `exclusions` contains JMESPath expressions to exclude from zero-omission.
// `currentPath` is the current JSON path being processed.
func prune(v reflect.Value, forceKeep bool, exclusions []string, currentPath string) (any, bool) {
	if !v.IsValid() {
		return nil, true
	}

	// Allow custom IsZero() overrides.
	if v.CanInterface() {
		if z, ok := v.Interface().(interface{ IsZero() bool }); ok && z.IsZero() && !forceKeep {
			return nil, true
		}
	}

	switch v.Kind() {
	case reflect.Pointer, reflect.Interface:
		if v.IsNil() {
			if forceKeep {
				return nil, false
			}
			return nil, true
		}
		return prune(v.Elem(), forceKeep, exclusions, currentPath)

	case reflect.Struct:
		out := make(map[string]any)
		allZero := true
		vt := v.Type()

		for i := 0; i < v.NumField(); i++ {
			sf := vt.Field(i)
			if sf.PkgPath != "" { // unexported
				continue
			}

			jName, jOpts := parseJSONTag(sf.Tag.Get("json"))
			if jName == "-" {
				continue
			}
			if jName == "" {
				jName = sf.Name
			}

			// Build path for this field
			fieldPath := currentPath
			if fieldPath == "" {
				fieldPath = jName
			} else {
				fieldPath = fieldPath + "." + jName
			}

			childForce := forceKeep || hasNoOmit(sf.Tag, jOpts) || matchesExclusion(fieldPath, exclusions)
			val, zero := prune(v.Field(i), childForce, exclusions, fieldPath)
			if zero && !childForce {
				continue
			}
			allZero = false
			out[jName] = val
		}

		if allZero && !forceKeep {
			return nil, true
		}
		return out, false

	case reflect.Slice, reflect.Array:
		if v.Len() == 0 && !forceKeep {
			return nil, true
		}
		arr := make([]any, 0, v.Len())
		allZero := true
		for i := 0; i < v.Len(); i++ {
			// Build path for array element
			elementPath := fmt.Sprintf("%s[%d]", currentPath, i)
			if currentPath == "" {
				elementPath = fmt.Sprintf("[%d]", i)
			}

			elementForce := matchesExclusion(elementPath, exclusions)
			val, zero := prune(v.Index(i), elementForce, exclusions, elementPath)
			arr = append(arr, val)
			if !zero {
				allZero = false
			}
		}
		if allZero && !forceKeep {
			return nil, true
		}
		return arr, false

	case reflect.Map:
		if v.Len() == 0 && !forceKeep {
			return nil, true
		}
		m := make(map[string]any)
		allZero := true
		for _, k := range v.MapKeys() {
			if k.Kind() != reflect.String { // JSON maps need string keys
				continue
			}

			// Build path for map key
			keyPath := currentPath
			if keyPath == "" {
				keyPath = k.String()
			} else {
				keyPath = keyPath + "." + k.String()
			}

			keyForce := matchesExclusion(keyPath, exclusions)
			val, zero := prune(v.MapIndex(k), keyForce, exclusions, keyPath)
			if zero && !keyForce {
				continue
			}
			allZero = false
			m[k.String()] = val
		}
		if allZero && !forceKeep {
			return nil, true
		}
		return m, false

	case reflect.Bool:
		if !v.Bool() && !forceKeep {
			return nil, true
		}
		return v.Bool(), false
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		if v.Int() == 0 && !forceKeep {
			return nil, true
		}
		return v.Int(), false
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		if v.Uint() == 0 && !forceKeep {
			return nil, true
		}
		return v.Uint(), false
	case reflect.Float32, reflect.Float64:
		if v.Float() == 0 && !forceKeep {
			return nil, true
		}
		return v.Float(), false
	case reflect.String:
		if v.Len() == 0 && !forceKeep {
			return nil, true
		}
		return v.String(), false
	default: // chan, func, unsafe pointers, etc.
		return v.Interface(), false
	}
}

// ---------------- tag parsing ----------------

func parseJSONTag(tag string) (name string, opts tagOpts) {
	if idx := strings.Index(tag, ","); idx != -1 {
		return tag[:idx], tagOpts(tag[idx+1:])
	}
	return tag, tagOpts("")
}

type tagOpts string

func (o tagOpts) contains(opt string) bool {
	if len(o) == 0 {
		return false
	}
	s := string(o)
	for s != "" {
		var next string
		i := strings.Index(s, ",")
		if i >= 0 {
			s, next = s[:i], s[i+1:]
		}
		if s == opt {
			return true
		}
		s = next
	}
	return false
}

// hasNoOmit returns true if either a dedicated no_omit tag exists
// OR the json tag contains ",no_omit".
func hasNoOmit(tag reflect.StructTag, jOpts tagOpts) bool {
	if tag.Get("no_omit") != "" {
		return true
	}
	return jOpts.contains("no_omit")
}
