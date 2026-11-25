package wsrelated

import (
	"reflect"
	"strings"
)

// TagKey is the struct tag that records which include_related path populates
// a field of codersdk.Workspace or one of its sub-structs.
//
// Within a struct that has any wsrelated tags, each tagged field is returned
// when its tag is selected, and untagged fields are not returned as part of a
// workspace. An empty tag means the field is always returned. A tag listing
// several comma-separated paths means every listed path must be selected. A
// struct with no wsrelated tags is covered entirely by the tag on the field
// that contains it.
const TagKey = "wsrelated"

// Field is a field reachable from a root struct that is returned as part of a
// workspace.
type Field struct {
	// GoPath is the chain of Go field names from the root struct.
	GoPath []string
	// JSONPath is the dotted chain of JSON names from the root struct. An
	// element of a slice or map is written as "[]", as in
	// "latest_build.resources[].agents".
	JSONPath string
	// Tag is the include_related value that populates the field.
	Tag string
	// Inherited reports that the tag comes from the field that contains this
	// field's struct, because that struct has no wsrelated tags.
	Inherited bool
	// Struct is the struct type that declares the field.
	Struct reflect.Type
}

// Fields walks root, a struct type, and returns every field that is returned
// as part of a workspace, parents before their children. It descends into
// struct, pointer, slice, and map types declared in the same package as root.
// Fields that JSON encoding omits are skipped.
func Fields(root reflect.Type) []Field {
	return collectFields(root, root.PkgPath(), nil, "", "")
}

func collectFields(t reflect.Type, pkgPath string, goPath []string, jsonPath, parentTag string) []Field {
	tagged := false
	for i := range t.NumField() {
		if _, ok := t.Field(i).Tag.Lookup(TagKey); ok {
			tagged = true
			break
		}
	}

	var fields []Field
	for i := range t.NumField() {
		sf := t.Field(i)
		if !sf.IsExported() {
			continue
		}
		jsonName, _, _ := strings.Cut(sf.Tag.Get("json"), ",")
		if jsonName == "-" {
			continue
		}
		if jsonName == "" {
			jsonName = sf.Name
		}

		tag := parentTag
		if tagged {
			var ok bool
			tag, ok = sf.Tag.Lookup(TagKey)
			if !ok {
				continue
			}
		}

		field := Field{
			GoPath:    append(append([]string{}, goPath...), sf.Name),
			JSONPath:  jsonName,
			Tag:       tag,
			Inherited: !tagged,
			Struct:    t,
		}
		if jsonPath != "" {
			field.JSONPath = jsonPath + "." + jsonName
		}
		fields = append(fields, field)

		elem := sf.Type
		childJSONPath := field.JSONPath
		for elem.Kind() == reflect.Pointer || elem.Kind() == reflect.Slice || elem.Kind() == reflect.Map {
			if elem.Kind() != reflect.Pointer {
				childJSONPath += "[]"
			}
			elem = elem.Elem()
		}
		if elem.Kind() == reflect.Struct && elem.PkgPath() == pkgPath {
			fields = append(fields, collectFields(elem, pkgPath, field.GoPath, childJSONPath, tag)...)
		}
	}
	return fields
}
