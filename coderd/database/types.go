package database

import (
	"database/sql/driver"
	"encoding/json"
	"time"

	"golang.org/x/xerrors"

	"github.com/coder/coder/coderd/rbac"
)

type Actions []rbac.Action

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
type TemplateACL map[string][]rbac.Action

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

// Now returns a standardized timezone used for database resources.
func Now() time.Time {
	return Time(time.Now().UTC())
}

// Time returns a time compatible with Postgres. Postgres only stores dates with
// microsecond precision.
func Time(t time.Time) time.Time {
	return t.Round(time.Microsecond)
}
