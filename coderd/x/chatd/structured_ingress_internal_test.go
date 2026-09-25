package chatd

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/x/chatd/chatstructured"
)

func TestParseResponseFormat(t *testing.T) {
	t.Parallel()
	schema := `{"type":"object","properties":{"SCHEMA_MARKER":{"type":"string"}}}`
	valid := `{"type":"json_schema","json_schema":{"name":"report","description":"d","schema":` + schema + `}}`
	withSpec := func(spec string) string { return `{"type":"json_schema","json_schema":` + spec + `}` }
	for _, tt := range []struct {
		name     string
		raw      string
		disabled bool
		field    string // empty for no rejection
		ordinary bool
	}{
		{name: "Omitted", ordinary: true},
		{name: "Null", raw: `null`, ordinary: true},
		{name: "Text", raw: `{"type":"text"}`, ordinary: true},
		{name: "Disabled", raw: valid, disabled: true, field: "response_format"},
		{name: "DuplicateKey", raw: `{"type":"json_schema","type":"text"}`, field: "response_format"},
		{name: "UnknownKey", raw: `{"type":"text","strict":true}`, field: "response_format"},
		{name: "SchemaWithText", raw: `{"type":"text","json_schema":{}}`, field: "response_format"},
		{name: "Empty", raw: `{}`, field: "response_format.type"},
		{name: "UnknownType", raw: `{"type":"xml"}`, field: "response_format.type"},
		{name: "MissingSpec", raw: `{"type":"json_schema"}`, field: "response_format.json_schema"},
		{name: "StrictSpec", raw: withSpec(`{"name":"r","schema":{},"strict":true}`), field: "response_format.json_schema"},
		{name: "MissingName", raw: withSpec(`{"schema":{}}`), field: "response_format.json_schema.name"},
		{name: "BadName", raw: withSpec(`{"name":"a b","schema":{}}`), field: "response_format.json_schema.name"},
		{name: "LongDescription", raw: withSpec(`{"name":"r","description":"` + strings.Repeat("d", 1025) + `","schema":{}}`), field: "response_format.json_schema.description"},
		{name: "InvalidUTF8", raw: withSpec("{\"name\":\"r\",\"description\":\"\xff\",\"schema\":{}}"), field: "response_format.json_schema.description"},
		{name: "MissingSchema", raw: withSpec(`{"name":"r"}`), field: "response_format.json_schema.schema"},
		{name: "SchemaNotObject", raw: withSpec(`{"name":"r","schema":true}`), field: "response_format.json_schema.schema"},
		{name: "SchemaInvalid", raw: withSpec(`{"name":"r","schema":{"type":"SCHEMA_MARKER"}}`), field: "response_format.json_schema.schema"},
		{name: "Valid", raw: valid},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			part, rejection := ParseResponseFormat(json.RawMessage(tt.raw), !tt.disabled)
			switch {
			case tt.field != "":
				require.Nil(t, part)
				require.NotNil(t, rejection)
				require.Equal(t, tt.field, rejection.Field)
				require.NotEmpty(t, rejection.Detail)
				require.NotContains(t, rejection.Detail, "SCHEMA_MARKER")
			case tt.ordinary:
				require.Nil(t, part)
				require.Nil(t, rejection)
			default:
				require.Nil(t, rejection)
				require.NotNil(t, part)
				request, err := chatstructured.DecodeRequestPart(*part)
				require.NoError(t, err)
				require.NotEqual(t, uuid.Nil, request.RequestID)
				require.Equal(t, "report", request.Name)
				require.Equal(t, "d", request.Description)
				require.JSONEq(t, schema, string(request.Schema))
			}
		})
	}
}
