package database_test

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/coder/coder/coderd/database"
)

// TestViewSubsetTemplate ensures TemplateTable is a subset of Template
func TestViewSubsetTemplate(t *testing.T) {
	t.Parallel()
	table := reflect.TypeOf(database.TemplateTable{})
	joined := reflect.TypeOf(database.Template{})

	tableFields := allFields(table)
	joinedFields := allFields(joined)
	if !assert.Subset(t, fieldNames(joinedFields), fieldNames(tableFields), "table is not subset") {
		t.Log("Some fields were added to the Template Table without updating the 'template_with_users' view.")
		t.Log("See migration 000138_join_users.up.sql to create the view.")
	}
}

func fieldNames(fields []reflect.StructField) []string {
	names := make([]string, len(fields))
	for i, field := range fields {
		names[i] = field.Name
	}
	return names
}

func allFields(rt reflect.Type) []reflect.StructField {
	fields := make([]reflect.StructField, 0, rt.NumField())
	for i := 0; i < rt.NumField(); i++ {
		field := rt.Field(i)
		if field.Anonymous && field.Type.Kind() == reflect.Struct {
			// Recurse into anonymous struct fields.
			fields = append(fields, allFields(field.Type)...)
			continue
		}
		fields = append(fields, rt.Field(i))
	}
	return fields
}
