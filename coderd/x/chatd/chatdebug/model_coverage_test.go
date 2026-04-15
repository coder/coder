package chatdebug //nolint:testpackage // Checks unexported normalized structs against fantasy source types.

import (
	"reflect"
	"testing"

	"charm.land/fantasy"
	"github.com/stretchr/testify/require"
)

// fieldDisposition documents whether a fantasy struct field is captured
// by the corresponding normalized struct ("normalized") or
// intentionally omitted ("skipped: <reason>"). The test fails when a
// fantasy type gains a field that is not yet classified, forcing the
// developer to decide whether to normalize or skip it.
//
// This mirrors the audit-table exhaustiveness check in
// enterprise/audit/table.go; same idea, different domain.
type fieldDisposition = map[string]string

// TestNormalizationFieldCoverage ensures every exported field on the
// fantasy types that model.go normalizes is explicitly accounted for.
// When the fantasy library adds a field the test fails, surfacing the
// drift at `go test` time rather than silently dropping data.
func TestNormalizationFieldCoverage(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		typ    reflect.Type
		fields fieldDisposition
	}{
		// ── struct-to-struct mappings ──────────────────────────

		{
			name: "fantasy.Usage → normalizedUsage",
			typ:  reflect.TypeFor[fantasy.Usage](),
			fields: fieldDisposition{
				"InputTokens":         "normalized",
				"OutputTokens":        "normalized",
				"TotalTokens":         "normalized",
				"ReasoningTokens":     "normalized",
				"CacheCreationTokens": "normalized",
				"CacheReadTokens":     "normalized",
			},
		},
		{
			name: "fantasy.Call → normalizedCallPayload",
			typ:  reflect.TypeFor[fantasy.Call](),
			fields: fieldDisposition{
				"Prompt":           "normalized",
				"MaxOutputTokens":  "normalized",
				"Temperature":      "normalized",
				"TopP":             "normalized",
				"TopK":             "normalized",
				"PresencePenalty":  "normalized",
				"FrequencyPenalty": "normalized",
				"Tools":            "normalized",
				"ToolChoice":       "normalized",
				"UserAgent":        "skipped: internal transport header, not useful for debug panel",
				"ProviderOptions":  "skipped: opaque provider data, only count preserved",
			},
		},
		{
			name: "fantasy.ObjectCall → normalizedObjectCallPayload",
			typ:  reflect.TypeFor[fantasy.ObjectCall](),
			fields: fieldDisposition{
				"Prompt":            "normalized",
				"Schema":            "skipped: full schema too large; SchemaName+SchemaDescription captured instead",
				"SchemaName":        "normalized",
				"SchemaDescription": "normalized",
				"MaxOutputTokens":   "normalized",
				"Temperature":       "normalized",
				"TopP":              "normalized",
				"TopK":              "normalized",
				"PresencePenalty":   "normalized",
				"FrequencyPenalty":  "normalized",
				"UserAgent":         "skipped: internal transport header, not useful for debug panel",
				"ProviderOptions":   "skipped: opaque provider data, only count preserved",
				"RepairText":        "skipped: function value, not serializable",
			},
		},
		{
			name: "fantasy.Response → normalizedResponsePayload",
			typ:  reflect.TypeFor[fantasy.Response](),
			fields: fieldDisposition{
				"Content":          "normalized",
				"FinishReason":     "normalized",
				"Usage":            "normalized",
				"Warnings":         "normalized",
				"ProviderMetadata": "skipped: opaque provider-specific metadata",
			},
		},
		{
			name: "fantasy.ObjectResponse → normalizedObjectResponsePayload",
			typ:  reflect.TypeFor[fantasy.ObjectResponse](),
			fields: fieldDisposition{
				"Object":           "skipped: arbitrary user type, not serializable generically",
				"RawText":          "normalized: as RawTextLength (length only, content unbounded)",
				"Usage":            "normalized",
				"FinishReason":     "normalized",
				"Warnings":         "normalized",
				"ProviderMetadata": "skipped: opaque provider-specific metadata",
			},
		},
		{
			name: "fantasy.CallWarning → normalizedWarning",
			typ:  reflect.TypeFor[fantasy.CallWarning](),
			fields: fieldDisposition{
				"Type":    "normalized",
				"Setting": "normalized",
				"Tool":    "skipped: interface value, warning message+type sufficient for debug panel",
				"Details": "normalized",
				"Message": "normalized",
			},
		},
		{
			name: "fantasy.StreamPart → appendNormalizedStreamContent",
			typ:  reflect.TypeFor[fantasy.StreamPart](),
			fields: fieldDisposition{
				"Type":             "normalized",
				"ID":               "normalized: as ToolCallID in content parts",
				"ToolCallName":     "normalized: as ToolName in content parts",
				"ToolCallInput":    "normalized: as Arguments or Result (bounded)",
				"Delta":            "normalized: accumulated into text/reasoning content parts",
				"ProviderExecuted": "skipped: provider vs client distinction not needed for debug panel",
				"Usage":            "normalized: captured in stream finalize",
				"FinishReason":     "normalized: captured in stream finalize",
				"Error":            "normalized: captured in stream error handling",
				"Warnings":         "normalized: captured in stream warning accumulation",
				"SourceType":       "normalized",
				"URL":              "normalized",
				"Title":            "normalized",
				"ProviderMetadata": "skipped: opaque provider-specific metadata",
			},
		},
		{
			name: "fantasy.ObjectStreamPart → wrapObjectStreamSeq",
			typ:  reflect.TypeFor[fantasy.ObjectStreamPart](),
			fields: fieldDisposition{
				"Type":             "normalized: drives switch in wrapObjectStreamSeq",
				"Object":           "skipped: arbitrary user type, only ObjectPartCount tracked",
				"Delta":            "normalized: accumulated into rawTextLength",
				"Error":            "normalized: captured in stream error handling",
				"Usage":            "normalized: captured in stream finalize",
				"FinishReason":     "normalized: captured in stream finalize",
				"Warnings":         "normalized: captured in stream warning accumulation",
				"ProviderMetadata": "skipped: opaque provider-specific metadata",
			},
		},

		// ── message part types (normalizeMessageParts) ────────

		{
			name: "fantasy.TextPart → normalizedMessagePart",
			typ:  reflect.TypeFor[fantasy.TextPart](),
			fields: fieldDisposition{
				"Text":            "normalized: bounded to MaxMessagePartTextLength",
				"ProviderOptions": "skipped: opaque provider-specific options",
			},
		},
		{
			name: "fantasy.ReasoningPart → normalizedMessagePart",
			typ:  reflect.TypeFor[fantasy.ReasoningPart](),
			fields: fieldDisposition{
				"Text":            "normalized: bounded to MaxMessagePartTextLength",
				"ProviderOptions": "skipped: opaque provider-specific options",
			},
		},
		{
			name: "fantasy.FilePart → normalizedMessagePart",
			typ:  reflect.TypeFor[fantasy.FilePart](),
			fields: fieldDisposition{
				"Filename":        "normalized",
				"Data":            "skipped: binary data never stored in debug records",
				"MediaType":       "normalized",
				"ProviderOptions": "skipped: opaque provider-specific options",
			},
		},
		{
			name: "fantasy.ToolCallPart → normalizedMessagePart",
			typ:  reflect.TypeFor[fantasy.ToolCallPart](),
			fields: fieldDisposition{
				"ToolCallID":       "normalized",
				"ToolName":         "normalized",
				"Input":            "normalized: as Arguments (bounded)",
				"ProviderExecuted": "skipped: provider vs client distinction not needed for debug panel",
				"ProviderOptions":  "skipped: opaque provider-specific options",
			},
		},
		{
			name: "fantasy.ToolResultPart → normalizedMessagePart",
			typ:  reflect.TypeFor[fantasy.ToolResultPart](),
			fields: fieldDisposition{
				"ToolCallID":       "normalized",
				"Output":           "normalized: text extracted via normalizeToolResultOutput",
				"ProviderExecuted": "skipped: provider vs client distinction not needed for debug panel",
				"ProviderOptions":  "skipped: opaque provider-specific options",
			},
		},

		// ── response content types (normalizeContentParts) ────

		{
			name: "fantasy.TextContent → normalizedContentPart",
			typ:  reflect.TypeFor[fantasy.TextContent](),
			fields: fieldDisposition{
				"Text":             "normalized: bounded to MaxMessagePartTextLength",
				"ProviderMetadata": "skipped: opaque provider-specific metadata",
			},
		},
		{
			name: "fantasy.ReasoningContent → normalizedContentPart",
			typ:  reflect.TypeFor[fantasy.ReasoningContent](),
			fields: fieldDisposition{
				"Text":             "normalized: bounded to MaxMessagePartTextLength",
				"ProviderMetadata": "skipped: opaque provider-specific metadata",
			},
		},
		{
			name: "fantasy.FileContent → normalizedContentPart",
			typ:  reflect.TypeFor[fantasy.FileContent](),
			fields: fieldDisposition{
				"MediaType":        "normalized",
				"Data":             "skipped: binary data never stored in debug records",
				"ProviderMetadata": "skipped: opaque provider-specific metadata",
			},
		},
		{
			name: "fantasy.SourceContent → normalizedContentPart",
			typ:  reflect.TypeFor[fantasy.SourceContent](),
			fields: fieldDisposition{
				"SourceType":       "normalized",
				"ID":               "skipped: provider-internal identifier, not actionable in debug panel",
				"URL":              "normalized",
				"Title":            "normalized",
				"MediaType":        "skipped: only relevant for document sources, rarely useful for debugging",
				"Filename":         "skipped: only relevant for document sources, rarely useful for debugging",
				"ProviderMetadata": "skipped: opaque provider-specific metadata",
			},
		},
		{
			name: "fantasy.ToolCallContent → normalizedContentPart",
			typ:  reflect.TypeFor[fantasy.ToolCallContent](),
			fields: fieldDisposition{
				"ToolCallID":       "normalized",
				"ToolName":         "normalized",
				"Input":            "normalized: as Arguments (bounded), InputLength tracks original",
				"ProviderExecuted": "skipped: provider vs client distinction not needed for debug panel",
				"ProviderMetadata": "skipped: opaque provider-specific metadata",
				"Invalid":          "skipped: validation state not surfaced in debug panel",
				"ValidationError":  "skipped: validation state not surfaced in debug panel",
			},
		},
		{
			name: "fantasy.ToolResultContent → normalizedContentPart",
			typ:  reflect.TypeFor[fantasy.ToolResultContent](),
			fields: fieldDisposition{
				"ToolCallID":       "normalized",
				"ToolName":         "normalized",
				"Result":           "normalized: text extracted via normalizeToolResultOutput",
				"ClientMetadata":   "skipped: client execution metadata not needed for debug panel",
				"ProviderExecuted": "skipped: provider vs client distinction not needed for debug panel",
				"ProviderMetadata": "skipped: opaque provider-specific metadata",
			},
		},

		// ── tool types (normalizeTools) ───────────────────────

		{
			name: "fantasy.FunctionTool → normalizedTool",
			typ:  reflect.TypeFor[fantasy.FunctionTool](),
			fields: fieldDisposition{
				"Name":            "normalized",
				"Description":     "normalized",
				"InputSchema":     "normalized: preserved as JSON for debug panel rendering",
				"ProviderOptions": "skipped: opaque provider-specific options",
			},
		},
		{
			name: "fantasy.ProviderDefinedTool → normalizedTool",
			typ:  reflect.TypeFor[fantasy.ProviderDefinedTool](),
			fields: fieldDisposition{
				"ID":   "normalized",
				"Name": "normalized",
				"Args": "skipped: provider-specific configuration not needed for debug panel",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// Every exported field on the fantasy type must be
			// registered as "normalized" or "skipped: <reason>".
			for i := range tt.typ.NumField() {
				field := tt.typ.Field(i)
				if !field.IsExported() {
					continue
				}
				disposition, ok := tt.fields[field.Name]
				if !ok {
					require.Failf(t, "unregistered field",
						"%s.%s is not in the coverage map: "+
							"add it as \"normalized\" or \"skipped: <reason>\"",
						tt.typ.Name(), field.Name)
				}
				require.NotEmptyf(t, disposition,
					"%s.%s has an empty disposition: "+
						"use \"normalized\" or \"skipped: <reason>\"",
					tt.typ.Name(), field.Name)
			}

			// Catch stale entries that reference removed fields.
			for name := range tt.fields {
				found := false
				for i := range tt.typ.NumField() {
					if tt.typ.Field(i).Name == name {
						found = true
						break
					}
				}
				require.Truef(t, found,
					"stale coverage entry %s.%s: "+
						"field no longer exists in fantasy, remove it",
					tt.typ.Name(), name)
			}
		})
	}
}
