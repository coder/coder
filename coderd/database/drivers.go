package database

import (
	"database/sql/driver"
	"encoding/json"

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
