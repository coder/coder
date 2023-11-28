package gentest_test

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/codersdk"
)

// TestAuditDBEnumsCovered ensures that all enums in the database are covered by the codersdk enums
// for audit log strings.
func TestAuditDBEnumsCovered(t *testing.T) {
	t.Parallel()

	dbTypes := database.AllResourceTypeValues()
	for _, ty := range dbTypes {
		str := codersdk.ResourceType(ty).FriendlyString()
		require.NotEqualf(t, "unknown", str, "ResourceType %q not covered by codersdk.ResourceType", ty)
	}
}

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

// TestViewSubsetTemplateVersion ensures TemplateVersionTable is a subset TemplateVersion
func TestViewSubsetTemplateVersion(t *testing.T) {
	t.Parallel()
	table := reflect.TypeOf(database.TemplateVersionTable{})
	joined := reflect.TypeOf(database.TemplateVersion{})

	tableFields := allFields(table)
	joinedFields := allFields(joined)
	if !assert.Subset(t, fieldNames(joinedFields), fieldNames(tableFields), "table is not subset") {
		t.Log("Some fields were added to the TemplateVersion Table without updating the 'template_version_with_user' view.")
		t.Log("See migration 000141_join_users_build_version.up.sql to create the view.")
	}
}

// TestViewSubsetWorkspaceBuild ensures WorkspaceBuildTable is a subset of WorkspaceBuild
func TestViewSubsetWorkspaceBuild(t *testing.T) {
	t.Parallel()
	table := reflect.TypeOf(database.WorkspaceBuildTable{})
	joined := reflect.TypeOf(database.WorkspaceBuild{})

	tableFields := allFields(table)
	joinedFields := allFields(joined)
	if !assert.Subset(t, fieldNames(joinedFields), fieldNames(tableFields), "table is not subset") {
		t.Log("Some fields were added to the WorkspaceBuild Table without updating the 'workspace_build_with_user' view.")
		t.Log("See migration 000141_join_users_build_version.up.sql to create the view.")
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
