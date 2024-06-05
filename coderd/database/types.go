package database

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/rbac/policy"
	"github.com/coder/coder/v2/codersdk/healthsdk"
)

// AuditOAuthConvertState is never stored in the database. It is stored in a cookie
// clientside as a JWT. This type is provided for audit logging purposes.
type AuditOAuthConvertState struct {
	CreatedAt time.Time `db:"created_at" json:"created_at"`
	// The time at which the state string expires, a merge request times out if the user does not perform it quick enough.
	ExpiresAt     time.Time `db:"expires_at" json:"expires_at"`
	FromLoginType LoginType `db:"from_login_type" json:"from_login_type"`
	// The login type the user is converting to. Should be github or oidc.
	ToLoginType LoginType `db:"to_login_type" json:"to_login_type"`
	UserID      uuid.UUID `db:"user_id" json:"user_id"`
}

type HealthSettings struct {
	ID                    uuid.UUID                 `db:"id" json:"id"`
	DismissedHealthchecks []healthsdk.HealthSection `db:"dismissed_healthchecks" json:"dismissed_healthchecks"`
}

type Actions []policy.Action

func (a *Actions) Scan(src interface{}) error {
	switch v := src.(type) {
	case string:
		return json.Unmarshal([]byte(v), &a)
	case []byte:
		return json.Unmarshal(v, &a)
	}
	return xerrors.Errorf("unexpected type %T", src)
}

func (a *Actions) Value() (driver.Value, error) {
	return json.Marshal(a)
}

// TemplateACL is a map of ids to permissions.
type TemplateACL map[string][]policy.Action

func (t *TemplateACL) Scan(src interface{}) error {
	switch v := src.(type) {
	case string:
		return json.Unmarshal([]byte(v), &t)
	case []byte, json.RawMessage:
		//nolint
		return json.Unmarshal(v.([]byte), &t)
	}

	return xerrors.Errorf("unexpected type %T", src)
}

func (t TemplateACL) Value() (driver.Value, error) {
	return json.Marshal(t)
}

type ExternalAuthProvider struct {
	ID       string `json:"id"`
	Optional bool   `json:"optional,omitempty"`
}

type StringMap map[string]string

func (m *StringMap) Scan(src interface{}) error {
	if src == nil {
		return nil
	}
	switch src := src.(type) {
	case []byte:
		err := json.Unmarshal(src, m)
		if err != nil {
			return err
		}
	default:
		return xerrors.Errorf("unsupported Scan, storing driver.Value type %T into type %T", src, m)
	}
	return nil
}

func (m StringMap) Value() (driver.Value, error) {
	return json.Marshal(m)
}

type StringMapOfInt map[string]int64

func (m *StringMapOfInt) Scan(src interface{}) error {
	if src == nil {
		return nil
	}
	switch src := src.(type) {
	case []byte:
		err := json.Unmarshal(src, m)
		if err != nil {
			return err
		}
	default:
		return xerrors.Errorf("unsupported Scan, storing driver.Value type %T into type %T", src, m)
	}
	return nil
}

func (m StringMapOfInt) Value() (driver.Value, error) {
	return json.Marshal(m)
}

type CustomRolePermissions []CustomRolePermission

func (a *CustomRolePermissions) Scan(src interface{}) error {
	switch v := src.(type) {
	case string:
		return json.Unmarshal([]byte(v), &a)
	case []byte:
		return json.Unmarshal(v, &a)
	}
	return xerrors.Errorf("unexpected type %T", src)
}

func (a CustomRolePermissions) Value() (driver.Value, error) {
	return json.Marshal(a)
}

type CustomRolePermission struct {
	Negate       bool          `json:"negate"`
	ResourceType string        `json:"resource_type"`
	Action       policy.Action `json:"action"`
}

func (a CustomRolePermission) String() string {
	str := a.ResourceType + "." + string(a.Action)
	if a.Negate {
		return "-" + str
	}
	return str
}

// NameOrganizationPair is used as a lookup tuple for custom role rows.
type NameOrganizationPair struct {
	Name string `db:"name" json:"name"`
	// OrganizationID if unset will assume a null column value
	OrganizationID uuid.UUID `db:"organization_id" json:"organization_id"`
}

func (*NameOrganizationPair) Scan(_ interface{}) error {
	return xerrors.Errorf("this should never happen, type 'NameOrganizationPair' should only be used as a parameter")
}

func (a NameOrganizationPair) Value() (driver.Value, error) {
	if a.OrganizationID == uuid.Nil {
		return fmt.Sprintf(`('%s', NULL)`, a.Name), nil
	}

	return fmt.Sprintf(`(%s,%s)`, a.Name, a.OrganizationID.String()), nil
}
