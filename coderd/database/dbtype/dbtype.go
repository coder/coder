package dbtype

import (
	"database/sql/driver"
	"encoding/json"

	"golang.org/x/xerrors"
)

type Map map[string]string

func (m Map) Scan(src interface{}) error {
	if src == nil {
		return nil
	}
	switch src := src.(type) {
	case []byte:
		err := json.Unmarshal(src, &m)
		if err != nil {
			return err
		}
	default:
		return xerrors.Errorf("unsupported Scan, storing driver.Value type %T into type %T", src, m)
	}
	return nil
}

func (m Map) Value() (driver.Value, error) {
	return json.Marshal(m)
}
