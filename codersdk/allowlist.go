package codersdk

import (
	"encoding/json"
	"strings"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/rbac/policy"
	"github.com/coder/coder/v2/x/wildcard"
)

// APIAllowListTarget is a typed allow-list entry that marshals to a single string
// "<resource_type>:<id>" where "*" is used as a wildcard for either side.
type APIAllowListTarget struct {
	Type wildcard.Value[RBACResource]
	ID   wildcard.Value[uuid.UUID]
}

func AllowAllTarget() APIAllowListTarget {
	return APIAllowListTarget{}
}

func AllowTypeTarget(r RBACResource) APIAllowListTarget {
	return APIAllowListTarget{Type: wildcard.Of(r)}
}

func AllowResourceTarget(r RBACResource, id uuid.UUID) APIAllowListTarget {
	return APIAllowListTarget{Type: wildcard.Of(r), ID: wildcard.Of(id)}
}

// String returns the canonical string representation "<type>:<id>" with "*" wildcards.
func (t APIAllowListTarget) String() string {
	return t.Type.String() + ":" + t.ID.String()
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

	// Type
	if parts[0] != policy.WildcardSymbol {
		t.Type = wildcard.Of(RBACResource(parts[0]))
	}

	// ID
	if parts[1] != policy.WildcardSymbol {
		u, err := uuid.Parse(parts[1])
		if err != nil {
			return xerrors.Errorf("invalid %s ID (must be UUID): %q", parts[0], parts[1])
		}
		t.ID = wildcard.Of(u)
	}
	return nil
}

// Implement encoding.TextMarshaler/Unmarshaler for broader compatibility

func (t APIAllowListTarget) MarshalText() ([]byte, error) { return []byte(t.String()), nil }

func (t *APIAllowListTarget) UnmarshalText(b []byte) error {
	return t.UnmarshalJSON([]byte("\"" + string(b) + "\""))
}
