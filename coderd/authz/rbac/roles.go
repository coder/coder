package rbac

import (
	"database/sql"
	"database/sql/driver"
	"strings"
	"unsafe"

	"github.com/lib/pq"
)

// Role.
type Role string

// Roles is a slice of roles.
type Roles []Role

var _ driver.Valuer = (*Roles)(nil)
var _ sql.Scanner = (*Roles)(nil)

// pq.Array won't accept our type aliased slice, so we need to turn it into an
// []string before passing it in. We can do this safely by casting the
// unsafe.Pointer to an []string since that's the slice's base type.
func (rs *Roles) toStrings() *[]string {
	// Make sure r is always initialized.
	if rs == nil {
		rs = &Roles{}
	}

	return (*[]string)(unsafe.Pointer(rs))
}

// Value implements the driver.Valuer interface.
func (rs Roles) Value() (driver.Value, error) {
	return pq.Array(rs.toStrings()).Value()
}

// Scan implements the sql.Scanner interface.
func (rs *Roles) Scan(src interface{}) error {
	return pq.Array(rs.toStrings()).Scan(src)
}

// String returns a string representation of roles.
func (rs Roles) String() string {
	var strs []string
	for _, role := range rs {
		strs = append(strs, string(role))
	}

	return strings.Join(strs, ",")
}

func (rs Roles) Contains(tgt Role) bool {
	for _, r := range rs {
		if r == tgt {
			return true
		}
	}
	return false
}
