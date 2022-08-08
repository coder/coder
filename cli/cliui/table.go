package cliui

import (
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/jedib0t/go-pretty/v6/table"
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
// of structs (panics otherwise, even for empty slices). At least one field in
// the struct must have a `table:""` tag containing the name of the column in
// the outputted table.
//
// Nested structs are processed if the field has the `table:"$NAME,recursive"`
// tag and their fields will be named as `$PARENT_NAME $NAME`. If the tag is
// malformed or a field is marked as recursive but does not contain a struct or
// a pointer to a struct, this function will panic (even with an empty input
// slice).
//
// If sort is empty, the input order will be used. If filterColumns is empty or
// nil, all available columns are included.
func DisplayTable(out interface{}, sort string, filterColumns []string) (string, error) {
	v := reflect.Indirect(reflect.ValueOf(out))

	if v.Kind() != reflect.Slice {
		panic("DisplayTable called with a non-slice type")
	}

	headersRaw := typeToTableHeaders(v.Type().Elem())
	if len(headersRaw) == 0 {
		panic("no table tags found on the input type")
	}
	headers := make(table.Row, len(headersRaw))
	for i, header := range headersRaw {
		headers[i] = header
	}

	tw := Table()
	tw.AppendHeader(headers)
	tw.SetColumnConfigs(FilterTableColumns(headers, filterColumns))
	tw.SortBy([]table.SortBy{{
		Name: sort,
	}})

	// Write each struct to the table.
	for i := 0; i < v.Len(); i++ {
		// Format the row as a slice.
		rowMap := valueToTableMap(v.Index(i))
		rowSlice := make([]interface{}, len(headers))
		for i, h := range headersRaw {
			v, ok := rowMap[h]
			if !ok {
				v = nil
			}

			// Special type formatting.
			switch val := v.(type) {
			case time.Time:
				v = val.Format(time.Stamp)
			case *time.Time:
				if val != nil {
					v = val.Format(time.Stamp)
				}
			}

			rowSlice[i] = v
		}

		tw.AppendRow(table.Row(rowSlice))
	}

	return tw.Render(), nil
}

func parseTableStructTag(field reflect.StructField) (name string, recurse bool, ok bool) {
	tag, ok := field.Tag.Lookup("table")
	if !ok {
		return "", false, false
	}

	tagSplit := strings.Split(tag, ",")
	if len(tagSplit) == 0 || tagSplit[0] == "" {
		panic(fmt.Sprintf(`invalid table tag %q, name must be a non-empty string`, tag))
	}
	if len(tagSplit) > 2 || (len(tagSplit) == 2 && strings.TrimSpace(tagSplit[1]) != "recursive") {
		panic(fmt.Sprintf(`invalid table tag %q, must be a non-empty string, optionally followed by ",recursive"`, tag))
	}

	return tagSplit[0], len(tagSplit) == 2, true
}

func isStructOrStructPointer(t reflect.Type) bool {
	return t.Kind() == reflect.Struct || (t.Kind() == reflect.Pointer && t.Elem().Kind() == reflect.Struct)
}

// typeToTableHeaders converts a type to a slice of column names. If the given
// type is not a struct, this function will panic.
func typeToTableHeaders(t reflect.Type) []string {
	if !isStructOrStructPointer(t) {
		panic("typeToTableHeaders called with a non-struct or a non-pointer-to-a-struct type")
	}
	if t.Kind() == reflect.Pointer {
		t = t.Elem()
	}

	headers := []string{}
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		name, recurse, ok := parseTableStructTag(field)
		if !ok {
			continue
		}

		if recurse {
			// If we're recursing, it must be a struct or a pointer to a struct.
			if !isStructOrStructPointer(field.Type) {
				panic(fmt.Sprintf("invalid recursive table tag, field %q is not a struct or a pointer to a struct so we cannot recurse", field.Name))
			}

			// typeToTableHeaders can handle pointers.
			childNames := typeToTableHeaders(field.Type)
			for _, childName := range childNames {
				headers = append(headers, fmt.Sprintf("%s %s", name, childName))
			}
			continue
		}

		headers = append(headers, name)
	}

	return headers
}

// valueToTableMap converts a struct to a map of column name to value. If the
// given value is not a struct, this function will panic.
func valueToTableMap(val reflect.Value) map[string]interface{} {
	if !isStructOrStructPointer(val.Type()) {
		panic("valueToTableMap called with a non-struct or a non-pointer-to-a-struct type")
	}
	if val.Kind() == reflect.Pointer {
		if val.IsNil() {
			return nil
		}

		val = val.Elem()
	}

	row := map[string]interface{}{}
	for i := 0; i < val.NumField(); i++ {
		field := val.Type().Field(i)
		fieldVal := val.Field(i)
		name, recurse, ok := parseTableStructTag(field)
		if !ok {
			continue
		}

		// If the field is a struct, recursively process it.
		if recurse {
			// It must be a struct or a pointer to a struct.
			if !isStructOrStructPointer(val.Type()) {
				panic(fmt.Sprintf("invalid recursive table tag, field %q is not a struct or a pointer to a struct so we cannot recurse", field.Name))
			}

			// valueToTableMap does nothing on pointers so we don't need to
			// filter here.
			childMap := valueToTableMap(fieldVal)
			for childName, childValue := range childMap {
				row[fmt.Sprintf("%s %s", name, childName)] = childValue
			}
			continue
		}

		// Otherwise, we just use the field value.
		row[name] = val.Field(i).Interface()
	}

	return row
}
