package codersdk

import (
	"encoding/json"
	"strings"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/rbac/policy"
)

// APIAllowListTarget represents a single allow-list entry using the canonical
// string form "<resource_type>:<id>". The wildcard symbol "*" is treated as a
// permissive match for either side.
type APIAllowListTarget struct {
	Type RBACResource `json:"type"`
	ID   string       `json:"id"`
}

func AllowAllTarget() APIAllowListTarget {
	return APIAllowListTarget{Type: ResourceWildcard, ID: policy.WildcardSymbol}
}

func AllowTypeTarget(r RBACResource) APIAllowListTarget {
	return APIAllowListTarget{Type: r, ID: policy.WildcardSymbol}
}

func AllowResourceTarget(r RBACResource, id uuid.UUID) APIAllowListTarget {
	return APIAllowListTarget{Type: r, ID: id.String()}
}

// String returns the canonical string representation "<type>:<id>" with "*" wildcards.
func (t APIAllowListTarget) String() string {
	return string(t.Type) + ":" + t.ID
}

// MarshalJSON encodes as a JSON string: "<type>:<id>".
func (t APIAllowListTarget) MarshalJSON() ([]byte, error) {
	return json.Marshal(t.String())
}

// UnmarshalJSON decodes from a JSON string: "<type>:<id>".
func (t *APIAllowListTarget) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}
	parts := strings.SplitN(strings.TrimSpace(s), ":", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return xerrors.Errorf("invalid allow_list entry %q: want <type>:<id>", s)
	}

	resource, id := RBACResource(parts[0]), parts[1]

	// Type
	if resource != ResourceWildcard {
		if _, ok := policy.RBACPermissions[string(resource)]; !ok {
			return xerrors.Errorf("unknown resource type %q", resource)
		}
	}
	t.Type = resource

	// ID
	if id != policy.WildcardSymbol {
		if _, err := uuid.Parse(id); err != nil {
			return xerrors.Errorf("invalid %s ID (must be UUID): %q", resource, id)
		}
	}
	t.ID = id
	return nil
}

// Implement encoding.TextMarshaler/Unmarshaler for broader compatibility

func (t APIAllowListTarget) MarshalText() ([]byte, error) { return []byte(t.String()), nil }

func (t *APIAllowListTarget) UnmarshalText(b []byte) error {
	return t.UnmarshalJSON([]byte("\"" + string(b) + "\""))
}
