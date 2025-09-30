package codersdk

import (
	"encoding/json"
	"strings"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/rbac/policy"
)

// APIAllowListTarget represents a single allow-list entry. The canonical string
// form is "<resource_type>:<id>" with "*" acting as a wildcard for either
// component. Structured JSON callers should use the object form
// `{ "type": "workspace", "id": "<uuid>" }`. Optionally, servers may attach a
// DisplayName to provide a human-friendly label; clients must ignore the field
// when submitting data.
type APIAllowListTarget struct {
	Type        RBACResource `json:"type"`
	ID          string       `json:"id"`
	DisplayName string       `json:"display_name,omitempty"`
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

// String returns the canonical string representation "<type>:<id>" with wildcards preserved.
func (t APIAllowListTarget) String() string {
	return string(t.Type) + ":" + t.ID
}

// UnmarshalJSON accepts either the structured object representation
// `{ "type": "workspace", "id": "<uuid>" }` or the legacy string form "workspace:<uuid>".
func (t *APIAllowListTarget) UnmarshalJSON(b []byte) error {
	if len(b) == 0 {
		return xerrors.New("empty allow_list entry")
	}

	// Attempt to decode the structured object form first.
	var obj struct {
		Type        string `json:"type"`
		ID          string `json:"id"`
		DisplayName string `json:"display_name"`
	}
	if err := json.Unmarshal(b, &obj); err == nil {
		if obj.Type != "" || obj.ID != "" {
			if obj.Type == "" || obj.ID == "" {
				return xerrors.New("allow_list entry must include both type and id")
			}
			if err := t.setValues(obj.Type, obj.ID); err != nil {
				return err
			}
			// Ignore object.DisplayName on input to keep backend validation strict.
			return nil
		}
	}

	var legacy string
	if err := json.Unmarshal(b, &legacy); err != nil {
		return xerrors.New("invalid allow_list entry: expected object with type/id or string")
	}
	parts := strings.SplitN(strings.TrimSpace(legacy), ":", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return xerrors.Errorf("invalid allow_list entry %q: want <type>:<id>", legacy)
	}
	return t.setValues(parts[0], parts[1])
}

func (t *APIAllowListTarget) setValues(rawType, rawID string) error {
	rawType = strings.TrimSpace(rawType)
	rawID = strings.TrimSpace(rawID)

	if rawType == "" || rawID == "" {
		return xerrors.New("allow_list entry must include non-empty type and id")
	}

	if rawType == policy.WildcardSymbol {
		t.Type = ResourceWildcard
	} else {
		if _, ok := policy.RBACPermissions[rawType]; !ok {
			return xerrors.Errorf("unknown resource type %q", rawType)
		}
		t.Type = RBACResource(rawType)
	}

	if rawID == policy.WildcardSymbol {
		t.ID = policy.WildcardSymbol
		return nil
	}

	if _, err := uuid.Parse(rawID); err != nil {
		return xerrors.Errorf("invalid %s ID (must be UUID): %q", rawType, rawID)
	}
	t.ID = rawID
	return nil
}

// MarshalJSON ensures encoding/json uses the structured representation instead
// of the legacy colon-delimited string form.
func (t APIAllowListTarget) MarshalJSON() ([]byte, error) {
	type alias APIAllowListTarget
	return json.Marshal(alias(t))
}

// Implement encoding.TextMarshaler/Unmarshaler for broader compatibility.
func (t APIAllowListTarget) MarshalText() ([]byte, error) { return []byte(t.String()), nil }

func (t *APIAllowListTarget) UnmarshalText(b []byte) error {
	strTarget := strings.TrimSpace(string(b))
	parts := strings.SplitN(strTarget, ":", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return xerrors.Errorf("invalid allow_list entry %q: want <type>:<id>", strTarget)
	}
	return t.setValues(parts[0], parts[1])
}
