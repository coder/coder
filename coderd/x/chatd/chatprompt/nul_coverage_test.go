package chatprompt_test

import (
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprompt"
	"github.com/coder/coder/v2/codersdk"
)

// Probe values injected into string-bearing fields. The NUL probe's
// prefix "before" puts the NUL at byte 6, which rejection tests assert.
const (
	nulProbe             = "before\x00middle\uE000\uE001after"
	naturalSentinelProbe = "before\uE000\uE001after"
)

func TestNeedsNulEncodingInJSON(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		raw  json.RawMessage
		want bool
	}{
		{name: "clean", raw: json.RawMessage(`{"value":"clean"}`)},
		{name: "nul", raw: json.RawMessage(`{"value":"\u0000"}`), want: true},
		{name: "escaped nul text", raw: json.RawMessage(`{"value":"\\u0000"}`)},
		{name: "escaped backslash then nul", raw: json.RawMessage(`{"value":"\\\u0000"}`), want: true},
		{name: "ordinary unicode escape", raw: json.RawMessage(`{"value":"\u1234"}`)},
		{name: "truncated unicode escape", raw: json.RawMessage(`{"value":"\u12"}`), want: true},
		{name: "unicode escape without digits", raw: json.RawMessage(`{"value":"\u`), want: true},
		{name: "invalid unicode escape", raw: json.RawMessage(`{"value":"\uZZZZ"}`), want: true},
		{name: "backslash outside string", raw: json.RawMessage(`\`)},
		{name: "trailing backslash in string", raw: json.RawMessage(`{"value":"\`)},

		{name: "two escaped backslashes then nul text", raw: json.RawMessage(`{"value":"\\\\u0000"}`)},
		{name: "escaped sentinel", raw: json.RawMessage(`{"value":"\uE000"}`), want: true},
		{name: "literal sentinel", raw: json.RawMessage(`{"value":""}`), want: true},
		{name: "valid surrogate pair", raw: json.RawMessage(`{"value":"\uD83D\uDE00"}`)},
		{name: "mixed case surrogate pair", raw: json.RawMessage(`{"value":"\ud83D\uDe00"}`)},
		{name: "lone high surrogate", raw: json.RawMessage(`{"value":"\uD800"}`), want: true},
		{name: "lone low surrogate", raw: json.RawMessage(`{"value":"\udc00"}`), want: true},
		{name: "reversed surrogate pair", raw: json.RawMessage(`{"value":"\uDC00\uD800"}`), want: true},
		{name: "surrogate literal text", raw: json.RawMessage(`{"value":"\\uD800"}`)},
		{name: "object key", raw: json.RawMessage(`{"key\u0000":"value"}`), want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := chatprompt.NeedsNulEncodingInJSONForTest(tt.raw)
			require.Equalf(t, tt.want, got, "needsNulEncodingInJSON(%s)", tt.raw)
		})
	}
}

func TestEncodeNulInJSONPreservesCleanInput(t *testing.T) {
	t.Parallel()

	raw := json.RawMessage(`{"number":1.2300,"pair":"\uD83D\uDE00","literal":"\\u0000"}`)
	encoded := chatprompt.EncodeNulInJSONForTest(raw)
	require.Equal(t, raw, encoded, "clean JSON changed")
	require.Same(t, &raw[0], &encoded[0], "clean JSON did not preserve its backing array")
}

func TestEncodeNulInJSONNormalizesSurrogates(t *testing.T) {
	t.Parallel()

	raw := json.RawMessage(`{"high":"\uD800","low":"\uDC00","pair":"\uD83D\uDE00","literal":"\\uD800"}`)
	encoded := chatprompt.EncodeNulInJSONForTest(raw)
	require.Truef(t, json.Valid(encoded), "encoded invalid JSON: %s", encoded)
	var got map[string]string
	require.NoError(t, json.Unmarshal(encoded, &got))
	want := map[string]string{
		"high":    "�",
		"low":     "�",
		"pair":    "😀",
		"literal": `\uD800`,
	}
	require.Equal(t, want, got, "normalized JSON mismatch")
}

func TestEncodeNulInJSONPreservesInvalidInput(t *testing.T) {
	t.Parallel()

	raw := json.RawMessage(`{"value":"\uD800"} trailing`)
	encoded := chatprompt.EncodeNulInJSONForTest(raw)
	require.Equal(t, raw, encoded, "invalid JSON changed")
}

func TestChatMessagePartNaturalSentinelCoverage(t *testing.T) {
	t.Parallel()

	partType := reflect.TypeFor[codersdk.ChatMessagePart]()
	for i := range partType.NumField() {
		field := partType.Field(i)
		if _, transforms := chatMessagePartNULProbe(t, field); !transforms {
			continue
		}

		t.Run(field.Name, func(t *testing.T) {
			t.Parallel()

			part := reflect.New(partType).Elem()
			part.Field(i).Set(chatMessagePartNaturalSentinelProbeValue(t, field.Type))
			parts := []codersdk.ChatMessagePart{part.Interface().(codersdk.ChatMessagePart)}

			encoded, err := chatprompt.MarshalParts(parts)
			require.NoErrorf(t, err, "MarshalParts rejected natural sentinels in ChatMessagePart.%s (%s)", field.Name, field.Type)
			decoded, err := chatprompt.ParseContent(database.ChatMessage{
				Role:           database.ChatMessageRoleAssistant,
				Content:        encoded,
				ContentVersion: chatprompt.CurrentContentVersion,
			})
			require.NoErrorf(t, err, "ParseContent with natural sentinels in ChatMessagePart.%s (%s)", field.Name, field.Type)
			require.Equalf(t, parts, decoded, "natural sentinels in ChatMessagePart.%s did not round-trip", field.Name)
		})
	}
}

func TestChatMessagePartNULCoverage(t *testing.T) {
	t.Parallel()

	partType := reflect.TypeFor[codersdk.ChatMessagePart]()
	for i := range partType.NumField() {
		field := partType.Field(i)
		probe, transforms := chatMessagePartNULProbe(t, field)

		t.Run(field.Name, func(t *testing.T) {
			t.Parallel()

			part := reflect.New(partType).Elem()
			part.Field(i).Set(probe)
			parts := []codersdk.ChatMessagePart{part.Interface().(codersdk.ChatMessagePart)}

			want := reflect.New(partType).Elem()
			want.Field(i).Set(chatMessagePartNULProbeValue(t, field.Type))
			wantParts := []codersdk.ChatMessagePart{want.Interface().(codersdk.ChatMessagePart)}

			encoded, err := chatprompt.MarshalParts(parts)
			require.Equalf(t, wantParts, parts, "MarshalParts mutated caller-owned ChatMessagePart.%s data", field.Name)
			if err != nil {
				require.Truef(t, transforms, "MarshalParts rejected NUL-free ChatMessagePart.%s (%s): %v", field.Name, field.Type, err)
				require.ErrorContainsf(t, err, "chat message part 0", "MarshalParts rejection for ChatMessagePart.%s does not name the part", field.Name)
				require.ErrorContainsf(t, err, "field "+field.Name, "MarshalParts rejection for ChatMessagePart.%s does not name the field", field.Name)
				require.ErrorContainsf(t, err, "contains NUL at byte 6", "MarshalParts rejection for ChatMessagePart.%s does not locate the NUL", field.Name)
				return
			}
			if transforms {
				require.NotContainsf(t, string(encoded.RawMessage), `\u0000`, "ChatMessagePart.%s is neither rejected nor handled by encodeNulInParts", field.Name)
			}

			decoded, err := chatprompt.ParseContent(database.ChatMessage{
				Role:           database.ChatMessageRoleAssistant,
				Content:        encoded,
				ContentVersion: chatprompt.CurrentContentVersion,
			})
			require.NoErrorf(t, err, "ParseContent with ChatMessagePart.%s (%s)", field.Name, field.Type)
			require.Equalf(t, wantParts, decoded, "ChatMessagePart.%s is not handled by decodeNulInParts", field.Name)
			require.Equalf(t, wantParts, parts, "ParseContent mutated caller-owned ChatMessagePart.%s data through an encoded alias", field.Name)
		})
	}
}

func chatMessagePartNaturalSentinelProbeValue(t *testing.T, typ reflect.Type) reflect.Value {
	t.Helper()

	if typ == reflect.TypeFor[json.RawMessage]() {
		return reflect.ValueOf(json.RawMessage(`{"key":"value"}`))
	}
	if typ.Kind() == reflect.String {
		value := reflect.New(typ).Elem()
		value.SetString(naturalSentinelProbe)
		return value
	}
	if chatMessagePartNULTransforms(typ) {
		return chatMessagePartStringContainerValue(t, typ, naturalSentinelProbe)
	}

	t.Fatalf("no natural sentinel probe for ChatMessagePart field type %s", typ)
	return reflect.Value{}
}

func chatMessagePartNULProbe(t *testing.T, field reflect.StructField) (reflect.Value, bool) {
	t.Helper()

	transforms := chatMessagePartNULTransforms(field.Type)
	if transforms || chatMessagePartNULSafeType(field.Type) {
		return chatMessagePartNULProbeValue(t, field.Type), transforms
	}

	t.Fatalf(
		"ChatMessagePart.%s has unknown NUL coverage shape %s; classify the type and add a partNulFields entry if it can contain strings",
		field.Name,
		field.Type,
	)
	return reflect.Value{}, false
}

func chatMessagePartNULTransforms(typ reflect.Type) bool {
	if typ == reflect.TypeFor[json.RawMessage]() {
		return true
	}
	if typ.Kind() == reflect.String {
		return true
	}
	if typ.Kind() != reflect.Slice && typ.Kind() != reflect.Array {
		return false
	}
	return chatMessagePartNULStringContainer(typ.Elem())
}

func chatMessagePartNULStringContainer(typ reflect.Type) bool {
	if typ.Kind() == reflect.String {
		return true
	}
	if typ.Kind() != reflect.Slice && typ.Kind() != reflect.Array {
		return false
	}
	return chatMessagePartNULStringContainer(typ.Elem())
}

func chatMessagePartNULSafeType(typ reflect.Type) bool {
	switch typ {
	case reflect.TypeFor[bool](),
		reflect.TypeFor[int](),
		reflect.TypeFor[[]byte](),
		reflect.TypeFor[uuid.NullUUID](),
		reflect.TypeFor[*time.Time]():
		return true
	default:
		return false
	}
}

func chatMessagePartNULProbeValue(t *testing.T, typ reflect.Type) reflect.Value {
	t.Helper()

	if typ == reflect.TypeFor[json.RawMessage]() {
		return reflect.ValueOf(json.RawMessage(`{"key\u0000":"value\u0000"}`))
	}
	if typ == reflect.TypeFor[[][]string]() {
		return reflect.ValueOf([][]string{{nulProbe}, nil})
	}
	if typ.Kind() == reflect.String {
		value := reflect.New(typ).Elem()
		value.SetString(nulProbe)
		return value
	}
	if chatMessagePartNULTransforms(typ) {
		return chatMessagePartStringContainerValue(t, typ, nulProbe)
	}

	switch typ {
	case reflect.TypeFor[bool]():
		return reflect.ValueOf(true)
	case reflect.TypeFor[int]():
		return reflect.ValueOf(42)
	case reflect.TypeFor[[]byte]():
		return reflect.ValueOf([]byte{0, 1, 2})
	case reflect.TypeFor[uuid.NullUUID]():
		return reflect.ValueOf(uuid.NullUUID{UUID: uuid.MustParse("00000000-0000-0000-0000-000000000001"), Valid: true})
	case reflect.TypeFor[*time.Time]():
		return reflect.ValueOf(new(time.Date(2026, time.August, 25, 1, 2, 3, 0, time.UTC)))
	default:
		t.Fatalf("no probe value for recognized ChatMessagePart field type %s", typ)
		return reflect.Value{}
	}
}

func chatMessagePartStringContainerValue(t *testing.T, typ reflect.Type, probe string) reflect.Value {
	t.Helper()

	if typ.Kind() == reflect.String {
		value := reflect.New(typ).Elem()
		value.SetString(probe)
		return value
	}

	switch typ.Kind() {
	case reflect.Slice:
		value := reflect.MakeSlice(typ, 1, 1)
		value.Index(0).Set(chatMessagePartStringContainerValue(t, typ.Elem(), probe))
		return value
	case reflect.Array:
		if typ.Len() == 0 {
			t.Fatalf("cannot inject a string probe into zero-length string container %s", typ)
		}
		value := reflect.New(typ).Elem()
		value.Index(0).Set(chatMessagePartStringContainerValue(t, typ.Elem(), probe))
		return value
	default:
		t.Fatalf("unsupported recursive string container %s", typ)
		return reflect.Value{}
	}
}
