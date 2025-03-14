package database

import (
	"errors"
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/coder/coder/v2/coderd/rbac/policy"
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
	ID                    uuid.UUID `db:"id" json:"id"`
	DismissedHealthchecks []string  `db:"dismissed_healthchecks" json:"dismissed_healthchecks"`
}

type NotificationsSettings struct {
	ID             uuid.UUID `db:"id" json:"id"`
	NotifierPaused bool      `db:"notifier_paused" json:"notifier_paused"`
}
type Actions []policy.Action

func (a *Actions) Scan(src interface{}) error {
	switch v := src.(type) {
	case string:
		return json.Unmarshal([]byte(v), &a)
	case []byte:

		return json.Unmarshal(v, &a)
	}

	return fmt.Errorf("unexpected type %T", src)
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
	return fmt.Errorf("unexpected type %T", src)
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
		return fmt.Errorf("unsupported Scan, storing driver.Value type %T into type %T", src, m)
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
		return fmt.Errorf("unsupported Scan, storing driver.Value type %T into type %T", src, m)

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
	return fmt.Errorf("unexpected type %T", src)
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

	return fmt.Errorf("this should never happen, type 'NameOrganizationPair' should only be used as a parameter")
}
// Value returns the tuple **literal**
// To get the literal value to return, you can use the expression syntax in a psql

// shell.
//
//		SELECT ('customrole'::text,'ece79dac-926e-44ca-9790-2ff7c5eb6e0c'::uuid);
//	To see 'null' option. Using the nil uuid as null to avoid empty string literals for null.
//		SELECT ('customrole',00000000-0000-0000-0000-000000000000);
//

// This value is usually used as an array, NameOrganizationPair[]. You can see
// what that literal is as well, with proper quoting.
//
//	SELECT ARRAY[('customrole'::text,'ece79dac-926e-44ca-9790-2ff7c5eb6e0c'::uuid)];
func (a NameOrganizationPair) Value() (driver.Value, error) {
	return fmt.Sprintf(`(%s,%s)`, a.Name, a.OrganizationID.String()), nil
}
// AgentIDNamePair is used as a result tuple for workspace and agent rows.

type AgentIDNamePair struct {
	ID   uuid.UUID `db:"id" json:"id"`
	Name string    `db:"name" json:"name"`
}
func (p *AgentIDNamePair) Scan(src interface{}) error {
	var v string
	switch a := src.(type) {

	case []byte:
		v = string(a)
	case string:
		v = a

	default:
		return fmt.Errorf("unexpected type %T", src)
	}
	parts := strings.Split(strings.Trim(v, "()"), ",")
	if len(parts) != 2 {
		return errors.New("invalid format for AgentIDNamePair")
	}
	id, err := uuid.Parse(strings.TrimSpace(parts[0]))
	if err != nil {
		return err
	}
	p.ID, p.Name = id, strings.TrimSpace(parts[1])
	return nil
}
func (p AgentIDNamePair) Value() (driver.Value, error) {
	return fmt.Sprintf(`(%s,%s)`, p.ID.String(), p.Name), nil

}
// UserLinkClaims is the returned IDP claims for a given user link.
// These claims are fetched at login time. These are the claims that were
// used for IDP sync.
type UserLinkClaims struct {
	IDTokenClaims  map[string]interface{} `json:"id_token_claims"`

	UserInfoClaims map[string]interface{} `json:"user_info_claims"`
	// MergeClaims are computed in Golang. It is the result of merging
	// the IDTokenClaims and UserInfoClaims. UserInfoClaims take precedence.
	MergedClaims map[string]interface{} `json:"merged_claims"`
}
func (a *UserLinkClaims) Scan(src interface{}) error {
	switch v := src.(type) {
	case string:
		return json.Unmarshal([]byte(v), &a)
	case []byte:
		return json.Unmarshal(v, &a)
	}
	return fmt.Errorf("unexpected type %T", src)
}
func (a UserLinkClaims) Value() (driver.Value, error) {
	return json.Marshal(a)
}
