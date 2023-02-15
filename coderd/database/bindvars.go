package database

import (
	"database/sql/driver"
	"fmt"
	"reflect"
	"regexp"
	"strings"

	"github.com/google/uuid"
	"github.com/lib/pq"

	"github.com/jmoiron/sqlx/reflectx"

	"github.com/coder/coder/coderd/util/slice"
)

var nameRegex = regexp.MustCompile(`@([a-zA-Z0-9_]+)`)
var dbmapper = reflectx.NewMapper("db")
var sqlValuer = reflect.TypeOf((*driver.Valuer)(nil)).Elem()

// bindNamed is an implementation that improves on the SQLx implementation. This
// adjusts the query to use "$#" syntax for arguments instead of "@argument". The
// returned args are the values of the struct fields that match the names in the
// correct order and indexing.
//
// 1. SQLx does not reuse arguments, so "@arg, @arg" will result in two arguments
// "$1, $2" instead of "$1, $1".
// 2. SQLx does not handle generic array types
// 3. SQLx only supports ":name" style arguments and breaks "::" type casting.
func bindNamed(query string, arg interface{}) (newQuery string, args []interface{}, err error) {
	// We do not need to implement a sql parser to extract and replace the variable names.
	// All names follow a simple regex.
	names := nameRegex.FindAllString(query, -1)
	// Get all unique names
	names = slice.Unique(names)

	// Replace all names with the correct index
	for i, name := range names {
		rpl := fmt.Sprintf("$%d", i+1)
		query = strings.ReplaceAll(query, name, rpl)
		names[i] = strings.TrimPrefix(name, "@")
	}

	arglist := make([]interface{}, 0, len(names))

	// This comes straight from SQLx
	// grab the indirected value of arg
	v := reflect.ValueOf(arg)
	for v = reflect.ValueOf(arg); v.Kind() == reflect.Ptr; {
		v = v.Elem()
	}

	err = dbmapper.TraversalsByNameFunc(v.Type(), names, func(i int, t []int) error {
		if len(t) == 0 {
			return fmt.Errorf("could not find name %s in %#v", names[i], arg)
		}

		val := reflectx.FieldByIndexesReadOnly(v, t)

		// Handle some custom types to make arguments easier to use.
		switch val.Interface().(type) {
		case []uuid.UUID:
			arglist = append(arglist, pq.Array(val.Interface()))
		default:
			arglist = append(arglist, val.Interface())
		}

		return nil
	})
	if err != nil {
		return "", nil, err
	}

	return query, arglist, nil
}

type UUIDs []uuid.UUID

func (ids UUIDs) Value() (driver.Value, error) {
	v := pq.Array(ids)
	return v.Value()
}

func (ids *UUIDs) Scan(src interface{}) error {
	v := pq.Array(ids)
	return v.Scan(src)
}
