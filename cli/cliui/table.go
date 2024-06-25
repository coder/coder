package cliui

import (
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/fatih/structtag"
	"github.com/jedib0t/go-pretty/v6/table"
	"golang.org/x/xerrors"
)

// Table creates a new table with standardized styles.
func Table() table.Writer {
	tableWriter := table.NewWriter()
	tableWriter.Style().Box.PaddingLeft = ""
	tableWriter.Style().Box.PaddingRight = "  "
	tableWriter.Style().Options.DrawBorder = false
	tableWriter.Style().Options.SeparateHeader = false
	tableWriter.Style().Options.SeparateColumns = false
	return tableWriter
}

// This type can be supplied as part of a slice to DisplayTable
// or to a `TableFormat` `Format` call to render a separator.
// Leading separators are not supported and trailing separators
// are ignored by the table formatter.
// e.g. `[]any{someRow, TableSeparator, someRow}`
type TableSeparator struct{}

// filterTableColumns returns configurations to hide columns
// that are not provided in the array. If the array is empty,
// no filtering will occur!
func filterTableColumns(header table.Row, columns []string) []table.ColumnConfig {
	if len(columns) == 0 {
		return nil
	}
	columnConfigs := make([]table.ColumnConfig, 0)
	for _, headerTextRaw := range header {
		headerText, _ := headerTextRaw.(string)
		hidden := true
		for _, column := range columns {
			if strings.EqualFold(strings.ReplaceAll(column, "_", " "), headerText) {
				hidden = false
				break
			}
		}
		columnConfigs = append(columnConfigs, table.ColumnConfig{
			Name:   headerText,
			Hidden: hidden,
		})
	}
	return columnConfigs
}

// DisplayTable renders a table as a string. The input argument can be:
//   - a struct slice.
//   - an interface slice, where the first element is a struct,
//     and all other elements are of the same type, or a TableSeparator.
//
// At least one field in the struct must have a `table:""` tag
// containing the name of the column in the outputted table.
//
// If `sort` is not specified, the field with the `table:"$NAME,default_sort"`
// tag will be used to sort. An error will be returned if no field has this tag.
//
// Nested structs are processed if the field has the `table:"$NAME,recursive"`
// tag and their fields will be named as `$PARENT_NAME $NAME`. If the tag is
// malformed or a field is marked as recursive but does not contain a struct or
// a pointer to a struct, this function will return an error (even with an empty
// input slice).
//
// If sort is empty, the input order will be used. If filterColumns is empty or
// nil, all available columns are included.
func DisplayTable(out any, sort string, filterColumns []string) (string, error) {
	v := reflect.Indirect(reflect.ValueOf(out))

	if v.Kind() != reflect.Slice {
		return "", xerrors.New("DisplayTable called with a non-slice type")
	}
	var tableType reflect.Type
	if v.Type().Elem().Kind() == reflect.Interface {
		if v.Len() == 0 {
			return "", xerrors.New("DisplayTable called with empty interface slice")
		}
		tableType = reflect.Indirect(reflect.ValueOf(v.Index(0).Interface())).Type()
	} else {
		tableType = v.Type().Elem()
	}

	// Get the list of table column headers.
	headersRaw, defaultSort, err := typeToTableHeaders(tableType, true)
	if err != nil {
		return "", xerrors.Errorf("get table headers recursively for type %q: %w", v.Type().Elem().String(), err)
	}
	if len(headersRaw) == 0 {
		return "", xerrors.New(`no table headers found on the input type, make sure there is at least one "table" struct tag`)
	}
	if sort == "" {
		sort = defaultSort
	}
	headers := make(table.Row, len(headersRaw))
	for i, header := range headersRaw {
		headers[i] = strings.ReplaceAll(header, "_", " ")
	}
	// Verify that the given sort column and filter columns are valid.
	if sort != "" || len(filterColumns) != 0 {
		headersMap := make(map[string]string, len(headersRaw))
		for _, header := range headersRaw {
			headersMap[strings.ToLower(header)] = header
		}

		if sort != "" {
			sort = strings.ToLower(strings.ReplaceAll(sort, "_", " "))
			h, ok := headersMap[sort]
			if !ok {
				return "", xerrors.Errorf(`specified sort column %q not found in table headers, available columns are "%v"`, sort, strings.Join(headersRaw, `", "`))
			}

			// Autocorrect
			sort = h
		}

		for i, column := range filterColumns {
			column := strings.ToLower(strings.ReplaceAll(column, "_", " "))
			h, ok := headersMap[column]
			if !ok {
				return "", xerrors.Errorf(`specified filter column %q not found in table headers, available columns are "%v"`, column, strings.Join(headersRaw, `", "`))
			}

			// Autocorrect
			filterColumns[i] = h
		}
	}

	// Verify that the given sort column is valid.
	if sort != "" {
		sort = strings.ReplaceAll(sort, "_", " ")
		found := false
		for _, header := range headersRaw {
			if strings.EqualFold(sort, header) {
				found = true
				sort = header
				break
			}
		}
		if !found {
			return "", xerrors.Errorf("specified sort column %q not found in table headers, available columns are %q", sort, strings.Join(headersRaw, `", "`))
		}
	}
	return renderTable(out, sort, headers, filterColumns)
}

func renderTable(out any, sort string, headers table.Row, filterColumns []string) (string, error) {
	v := reflect.Indirect(reflect.ValueOf(out))

	// Setup the table formatter.
	tw := Table()
	tw.AppendHeader(headers)
	tw.SetColumnConfigs(filterTableColumns(headers, filterColumns))
	if sort != "" {
		tw.SortBy([]table.SortBy{{
			Name: sort,
		}})
	}

	// Write each struct to the table.
	for i := 0; i < v.Len(); i++ {
		cur := v.Index(i).Interface()
		_, ok := cur.(TableSeparator)
		if ok {
			tw.AppendSeparator()
			continue
		}
		// Format the row as a slice.
		// ValueToTableMap does what `reflect.Indirect` does
		rowMap, err := valueToTableMap(reflect.ValueOf(cur))
		if err != nil {
			return "", xerrors.Errorf("get table row map %v: %w", i, err)
		}

		rowSlice := make([]any, len(headers))
		for i, h := range headers {
			v, ok := rowMap[h.(string)]
			if !ok {
				v = nil
			}

			// Special type formatting.
			switch val := v.(type) {
			case time.Time:
				v = val.Format(time.RFC3339)
			case *time.Time:
				if val != nil {
					v = val.Format(time.RFC3339)
				}
			case *int64:
				if val != nil {
					v = *val
				}
			case fmt.Stringer:
				if val != nil {
					v = val.String()
				}
			}

			// Guard against nil dereferences
			if v != nil {
				rt := reflect.TypeOf(v)
				switch rt.Kind() {
				case reflect.Slice:
					// By default, the behavior is '%v', which just returns a string like
					// '[a b c]'. This will add commas in between each value.
					strs := make([]string, 0)
					vt := reflect.ValueOf(v)
					for i := 0; i < vt.Len(); i++ {
						strs = append(strs, fmt.Sprintf("%v", vt.Index(i).Interface()))
					}
					v = "[" + strings.Join(strs, ", ") + "]"
				default:
					// Leave it as it is
				}
			}

			rowSlice[i] = v
		}

		tw.AppendRow(table.Row(rowSlice))
	}

	return tw.Render(), nil
}

// parseTableStructTag returns the name of the field according to the `table`
// struct tag. If the table tag does not exist or is "-", an empty string is
// returned. If the table tag is malformed, an error is returned.
//
// The returned name is transformed from "snake_case" to "normal text".
func parseTableStructTag(field reflect.StructField) (name string, defaultSort, noSortOpt, recursive, skipParentName bool, err error) {
	tags, err := structtag.Parse(string(field.Tag))
	if err != nil {
		return "", false, false, false, false, xerrors.Errorf("parse struct field tag %q: %w", string(field.Tag), err)
	}

	tag, err := tags.Get("table")
	if err != nil || tag.Name == "-" {
		// tags.Get only returns an error if the tag is not found.
		return "", false, false, false, false, nil
	}

	defaultSortOpt := false
	noSortOpt = false
	recursiveOpt := false
	skipParentNameOpt := false
	for _, opt := range tag.Options {
		switch opt {
		case "default_sort":
			defaultSortOpt = true
		case "nosort":
			noSortOpt = true
		case "recursive":
			recursiveOpt = true
		case "recursive_inline":
			// recursive_inline is a helper to make recursive tables look nicer.
			// It skips prefixing the parent name to the child name. If you do this,
			// make sure the child name is unique across all nested structs in the parent.
			recursiveOpt = true
			skipParentNameOpt = true
		default:
			return "", false, false, false, false, xerrors.Errorf("unknown option %q in struct field tag", opt)
		}
	}

	return strings.ReplaceAll(tag.Name, "_", " "), defaultSortOpt, noSortOpt, recursiveOpt, skipParentNameOpt, nil
}

func isStructOrStructPointer(t reflect.Type) bool {
	return t.Kind() == reflect.Struct || (t.Kind() == reflect.Pointer && t.Elem().Kind() == reflect.Struct)
}

// typeToTableHeaders converts a type to a slice of column names. If the given
// type is invalid (not a struct or a pointer to a struct, has invalid table
// tags, etc.), an error is returned.
//
// requireDefault is only needed for the root call. This is recursive, so nested
// structs do not need the default sort name.
// nolint:revive
func typeToTableHeaders(t reflect.Type, requireDefault bool) ([]string, string, error) {
	if !isStructOrStructPointer(t) {
		return nil, "", xerrors.Errorf("typeToTableHeaders called with a non-struct or a non-pointer-to-a-struct type")
	}
	if t.Kind() == reflect.Pointer {
		t = t.Elem()
	}

	headers := []string{}
	defaultSortName := ""
	noSortOpt := false
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		name, defaultSort, noSort, recursive, skip, err := parseTableStructTag(field)
		if err != nil {
			return nil, "", xerrors.Errorf("parse struct tags for field %q in type %q: %w", field.Name, t.String(), err)
		}
		if requireDefault && noSort {
			noSortOpt = true
		}

		if name == "" && (recursive && skip) {
			return nil, "", xerrors.Errorf("a name is required for the field %q. "+
				"recursive_line will ensure this is never shown to the user, but is still needed", field.Name)
		}
		// If recurse and skip is set, the name is intentionally empty.
		if name == "" {
			continue
		}
		if defaultSort {
			if defaultSortName != "" {
				return nil, "", xerrors.Errorf("multiple fields marked as default sort in type %q", t.String())
			}
			defaultSortName = name
		}

		fieldType := field.Type
		if recursive {
			if !isStructOrStructPointer(fieldType) {
				return nil, "", xerrors.Errorf("field %q in type %q is marked as recursive but does not contain a struct or a pointer to a struct", field.Name, t.String())
			}

			childNames, defaultSort, err := typeToTableHeaders(fieldType, false)
			if err != nil {
				return nil, "", xerrors.Errorf("get child field header names for field %q in type %q: %w", field.Name, fieldType.String(), err)
			}
			for _, childName := range childNames {
				fullName := fmt.Sprintf("%s %s", name, childName)
				if skip {
					fullName = childName
				}
				headers = append(headers, fullName)
			}
			if defaultSortName == "" {
				defaultSortName = defaultSort
			}
			continue
		}

		headers = append(headers, name)
	}

	if defaultSortName == "" && requireDefault && !noSortOpt {
		return nil, "", xerrors.Errorf("no field marked as default_sort or nosort in type %q", t.String())
	}

	return headers, defaultSortName, nil
}

// valueToTableMap converts a struct to a map of column name to value. If the
// given type is invalid (not a struct or a pointer to a struct, has invalid
// table tags, etc.), an error is returned.
func valueToTableMap(val reflect.Value) (map[string]any, error) {
	if !isStructOrStructPointer(val.Type()) {
		return nil, xerrors.Errorf("valueToTableMap called with a non-struct or a non-pointer-to-a-struct type")
	}
	if val.Kind() == reflect.Pointer {
		if val.IsNil() {
			// No data for this struct, so return an empty map. All values will
			// be rendered as nil in the resulting table.
			return map[string]any{}, nil
		}

		val = val.Elem()
	}

	row := map[string]any{}
	for i := 0; i < val.NumField(); i++ {
		field := val.Type().Field(i)
		fieldVal := val.Field(i)
		name, _, _, recursive, skip, err := parseTableStructTag(field)
		if err != nil {
			return nil, xerrors.Errorf("parse struct tags for field %q in type %T: %w", field.Name, val, err)
		}
		if name == "" {
			continue
		}

		// Recurse if it's a struct.
		fieldType := field.Type
		if recursive {
			if !isStructOrStructPointer(fieldType) {
				return nil, xerrors.Errorf("field %q in type %q is marked as recursive but does not contain a struct or a pointer to a struct", field.Name, fieldType.String())
			}

			// valueToTableMap does nothing on pointers so we don't need to
			// filter here.
			childMap, err := valueToTableMap(fieldVal)
			if err != nil {
				return nil, xerrors.Errorf("get child field values for field %q in type %q: %w", field.Name, fieldType.String(), err)
			}
			for childName, childValue := range childMap {
				fullName := fmt.Sprintf("%s %s", name, childName)
				if skip {
					fullName = childName
				}
				row[fullName] = childValue
			}
			continue
		}

		// Otherwise, we just use the field value.
		row[name] = val.Field(i).Interface()
	}

	return row, nil
}
