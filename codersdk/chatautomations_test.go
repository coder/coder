package codersdk_test

import (
	"reflect"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/codersdk"
)

// The API never returns a webhook secret except in the create response, so
// the types of every other response must have no field that could hold one.
func TestChatAutomationResponseTypesHaveNoSecret(t *testing.T) {
	t.Parallel()

	var secretFields []string
	var walk func(path string, typ reflect.Type)
	walk = func(path string, typ reflect.Type) {
		for typ.Kind() == reflect.Pointer || typ.Kind() == reflect.Slice {
			typ = typ.Elem()
		}
		if typ.Kind() != reflect.Struct {
			return
		}
		for i := range typ.NumField() {
			field := typ.Field(i)
			name := strings.ToLower(field.Name + " " + field.Tag.Get("json"))
			if strings.Contains(name, "secret") && field.Name != "WebhookSecretVersion" {
				secretFields = append(secretFields, path+"."+field.Name)
			}
			walk(path+"."+field.Name, field.Type)
		}
	}
	walk("ChatAutomation", reflect.TypeFor[codersdk.ChatAutomation]())
	require.Empty(t, secretFields)
}
