package chatstructured

import (
	"encoding/json"
	"regexp"
)

var requestNamePattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

// Request is the stored response_format json_schema object of a structured
// output request.
type Request struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Schema      json.RawMessage `json:"schema"`
}

// ValidRequestName reports whether name is a valid request name.
func ValidRequestName(name string) bool { return requestNamePattern.MatchString(name) }
