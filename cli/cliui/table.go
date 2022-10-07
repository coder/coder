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

// FilterTableColumns returns configurations to hide columns
// that are not provided in the array. If the array is empty,
// no filtering will occur!
func FilterTableColumns(header table.Row, columns []string) []table.ColumnConfig {
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

// DisplayTable renders a table as a string. The input argument must be a slice
// of structs. At least one field in the struct must have a `table:""` tag
// containing the name of the column in the outputted table.
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
		return "", xerrors.Errorf("DisplayTable called with a non-slice type")
	}

	// Get the list of table column headers.
	headersRaw, err := typeToTableHeaders(v.Type().Elem())
	if err != nil {
		return "", xerrors.Errorf("get table headers recursively for type %q: %w", v.Type().Elem().String(), err)
	}
	if len(headersRaw) == 0 {
		return "", xerrors.New(`no table headers found on the input type, make sure there is at least one "table" struct tag`)
	}
	headers := make(table.Row, len(headersRaw))
	for i, header := range headersRaw {
		headers[i] = header
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
				return "", xerrors.Errorf(`specified filter column %q not found in table headers, available columns are "%v"`, sort, strings.Join(headersRaw, `", "`))
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

	// Setup the table formatter.
	tw := Table()
	tw.AppendHeader(headers)
	tw.SetColumnConfigs(FilterTableColumns(headers, filterColumns))
	if sort != "" {
		tw.SortBy([]table.SortBy{{
			Name: sort,
		}})
	}

	// Write each struct to the table.
	for i := 0; i < v.Len(); i++ {
		// Format the row as a slice.
		rowMap, err := valueToTableMap(v.Index(i))
		if err != nil {
			return "", xerrors.Errorf("get table row map %v: %w", i, err)
		}

		rowSlice := make([]any, len(headers))
		for i, h := range headersRaw {
			v, ok := rowMap[h]
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
func parseTableStructTag(field reflect.StructField) (name string, recurse bool, err error) {
	tags, err := structtag.Parse(string(field.Tag))
	if err != nil {
		return "", false, xerrors.Errorf("parse struct field tag %q: %w", string(field.Tag), err)
	}

	tag, err := tags.Get("table")
	if err != nil || tag.Name == "-" {
		// tags.Get only returns an error if the tag is not found.
		return "", false, nil
	}

	recursive := false
	for _, opt := range tag.Options {
		if opt == "recursive" {
			recursive = true
			continue
		}

		return "", false, xerrors.Errorf("unknown option %q in struct field tag", opt)
	}

	return strings.ReplaceAll(tag.Name, "_", " "), recursive, nil
}

func isStructOrStructPointer(t reflect.Type) bool {
	return t.Kind() == reflect.Struct || (t.Kind() == reflect.Pointer && t.Elem().Kind() == reflect.Struct)
}

// typeToTableHeaders converts a type to a slice of column names. If the given
// type is invalid (not a struct or a pointer to a struct, has invalid table
// tags, etc.), an error is returned.
func typeToTableHeaders(t reflect.Type) ([]string, error) {
	if !isStructOrStructPointer(t) {
		return nil, xerrors.Errorf("typeToTableHeaders called with a non-struct or a non-pointer-to-a-struct type")
	}
	if t.Kind() == reflect.Pointer {
		t = t.Elem()
	}

	headers := []string{}
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		name, recursive, err := parseTableStructTag(field)
		if err != nil {
			return nil, xerrors.Errorf("parse struct tags for field %q in type %q: %w", field.Name, t.String(), err)
		}
		if name == "" {
			continue
		}

		fieldType := field.Type
		if recursive {
			if !isStructOrStructPointer(fieldType) {
				return nil, xerrors.Errorf("field %q in type %q is marked as recursive but does not contain a struct or a pointer to a struct", field.Name, t.String())
			}

			childNames, err := typeToTableHeaders(fieldType)
			if err != nil {
				return nil, xerrors.Errorf("get child field header names for field %q in type %q: %w", field.Name, fieldType.String(), err)
			}
			for _, childName := range childNames {
				headers = append(headers, fmt.Sprintf("%s %s", name, childName))
			}
			continue
		}

		headers = append(headers, name)
	}

	return headers, nil
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
		name, recursive, err := parseTableStructTag(field)
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
				row[fmt.Sprintf("%s %s", name, childName)] = childValue
			}
			continue
		}

		// Otherwise, we just use the field value.
		row[name] = val.Field(i).Interface()
	}

	return row, nil
}

// TableHeaders returns the table header names of all
// fields in tSlice. tSlice must be a slice of some type.
func TableHeaders(tSlice any) ([]string, error) {
	v := reflect.Indirect(reflect.ValueOf(tSlice))
	rawHeaders, err := typeToTableHeaders(v.Type().Elem())
	if err != nil {
		return nil, xerrors.Errorf("type to table headers: %w", err)
	}
	out := make([]string, 0, len(rawHeaders))
	for _, hdr := range rawHeaders {
		out = append(out, strings.Replace(hdr, " ", "_", -1))
	}
	return out, nil
}
