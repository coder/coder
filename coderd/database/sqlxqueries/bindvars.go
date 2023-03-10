package sqlxqueries

import (
	"fmt"
	"reflect"
	"regexp"
	"strings"

	"golang.org/x/xerrors"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx/reflectx"
	"github.com/lib/pq"

	"github.com/coder/coder/coderd/util/slice"
)

var nameRegex = regexp.MustCompile(`@([a-zA-Z0-9_]+)`)

// dbmapper grabs struct 'db' tags.
var dbmapper = reflectx.NewMapper("db")

// bindNamed is an implementation that improves on the SQLx implementation. This
// adjusts the query to use "$#" syntax for arguments instead of "@argument". The
// returned args are the values of the struct fields that match the names in the
// correct order and indexing.
//
// 1. SQLx does not reuse arguments, so "@arg, @arg" will result in two arguments
// "$1, $2" instead of "$1, $1".
// 2. SQLx does not handle uuid arrays.
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
		if strings.Contains(query, rpl) {
			return "", nil,
				xerrors.Errorf("query contains both named params %q, and unnamed %q: choose one", name, rpl)
		}
		query = strings.ReplaceAll(query, name, rpl)
		// Remove the "@" prefix to match to the "db" struct tag.
		names[i] = strings.TrimPrefix(name, "@")
	}

	arglist := make([]interface{}, 0, len(names))

	// This comes straight from SQLx's implementation to get the values
	// of the struct fields.
	var v reflect.Value
	for v = reflect.ValueOf(arg); v.Kind() == reflect.Ptr; {
		v = v.Elem()
	}

	// If there is only 1 argument, and the argument is not a struct, then
	// the only argument is the value passed in. This is a nice shortcut
	// for simple queries with 1 param like "id".
	if v.Type().Kind() != reflect.Struct && len(names) == 1 {
		arglist = append(arglist, pqValue(v))
		return query, arglist, nil
	}

	err = dbmapper.TraversalsByNameFunc(v.Type(), names, func(i int, t []int) error {
		if len(t) == 0 {
			return xerrors.Errorf("could not find name %s in %#v", names[i], arg)
		}

		val := reflectx.FieldByIndexesReadOnly(v, t)
		arglist = append(arglist, pqValue(val))

		return nil
	})
	if err != nil {
		return "", nil, err
	}

	return query, arglist, nil
}

func pqValue(val reflect.Value) interface{} {
	valI := val.Interface()
	// Handle some custom types to make arguments easier to use.
	switch valI.(type) {
	// Feel free to add more types here as needed.
	case []uuid.UUID:
		return pq.Array(valI)
	default:
		return valI
	}
}
