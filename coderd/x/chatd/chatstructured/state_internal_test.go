package chatstructured

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/codersdk"
)

func TestStructuredOutputCodecs(t *testing.T) {
	t.Parallel()
	id := uuid.New()
	// A schema the compiler rejects still decodes: history reads never compile.
	req := Request{RequestID: id, Name: "Report_v-2", Description: strings.Repeat("é", 512), Schema: json.RawMessage(`{"$ref":"#","maximum":9007199254740993}`)}
	part, err := EncodeRequestPart(req)
	require.NoError(t, err)
	require.Equal(t, codersdk.ChatMessagePartTypeStructuredOutputRequest, part.Type)
	gotReq, err := DecodeRequestPart(part)
	require.NoError(t, err)
	require.Equal(t, req, gotReq)
	_, err = CompileSchema(req.Schema)
	require.Error(t, err)

	for _, c := range []Control{
		{RequestID: id, Kind: ControlCandidate, Value: json.RawMessage(`{"n":9007199254740993,"s":"exact  text"}`)},
		{RequestID: id, Kind: ControlCandidate, Value: json.RawMessage(`null`)},
		{RequestID: id, Kind: ControlCandidate, Value: json.RawMessage(`[1e100,-1.5e-99]`)},
		{RequestID: id, Kind: ControlRejection},
		{RequestID: id, Kind: ControlInvalidation},
	} {
		part, err := EncodeControlPart(c)
		require.NoError(t, err)
		got, err := DecodeControlPart(part)
		require.NoError(t, err)
		require.Equal(t, c, got)
	}
	for _, o := range []codersdk.ChatStructuredOutput{
		{RequestID: id, Status: codersdk.ChatStructuredOutputStatusSucceeded, Value: json.RawMessage(`null`)},
		{RequestID: id, Status: codersdk.ChatStructuredOutputStatusFailed, Error: &codersdk.ChatStructuredOutputError{
			Code: codersdk.ChatStructuredOutputErrorCodeValidationExhausted, Message: "no valid output",
		}},
		{RequestID: id, Status: codersdk.ChatStructuredOutputStatusCanceled, Error: &codersdk.ChatStructuredOutputError{
			Code: codersdk.ChatStructuredOutputErrorCodeSuperseded, Message: "superseded",
		}},
	} {
		part, err := EncodeOutcomePart(o)
		require.NoError(t, err)
		got, err := DecodeOutcomePart(part)
		require.NoError(t, err)
		require.Equal(t, o, got)
	}

	// Every violation is the same fixed-text sentinel.
	r := `{"request_id":"` + id.String() + `",`
	nilID := `{"request_id":"` + uuid.Nil.String() + `",`
	for typ, payloads := range map[codersdk.ChatMessagePartType][]string{
		codersdk.ChatMessagePartTypeStructuredOutputRequest: {
			nilID + `"name":"a","schema":{}}`, r + `"name":"a","schema":null}`, r + `"name":"bad name","schema":{}}`, r + `"name":"` + strings.Repeat("a", 65) + `","schema":{}}`,
			r + `"name":"a","description":"` + strings.Repeat("a", 1025) + `","schema":{}}`, r + `"name":"a"}`, r + `"name":"a","schema":{},"x":1}`,
			r + `"NAME":"a","schema":{}}`, r + `"name":"a","schema":{}} {}`, r + `"name":"a","name":"b","schema":{}}`, `[]`, `null`, ``,
		},
		codersdk.ChatMessagePartTypeStructuredOutputControl: {
			nilID + `"kind":"rejection"}`, r + `"kind":"finish"}`, r + `"kind":"rejection","value":1}`, r + `"kind":"invalidation","value":null}`,
			r + `"kind":"candidate"}`, r + `"kind":"candidate","value":1,"x":1}`,
		},
		codersdk.ChatMessagePartTypeStructuredOutputOutcome: {
			r + `"status":"done","value":1}`, r + `"status":"succeeded"}`, r + `"status":"succeeded","value":1,"error":{"code":"not_produced","message":"m"}}`,
			r + `"status":"failed","error":{"code":"interrupted","message":"m"}}`, r + `"status":"failed","value":1,"error":{"code":"not_produced","message":"m"}}`,
			r + `"status":"failed"}`, r + `"status":"canceled","error":{"code":"not_produced","message":"m"}}`,
			r + `"status":"canceled","error":{"code":"interrupted","message":""}}`, r + `"status":"failed","error":{"code":"not_produced","message":"m","x":1}}`,
			r + `"status":"failed","error":{"code":"not_produced","message":"` + strings.Repeat("a", 1025) + `"}}`,
		},
	} {
		for _, payload := range payloads {
			_, err := decodePart(codersdk.ChatMessagePart{Type: typ, StructuredOutputData: json.RawMessage(payload)})
			require.ErrorIs(t, err, ErrMalformedStructuredOutputMetadata, payload)
			require.Equal(t, ErrMalformedStructuredOutputMetadata.Error(), err.Error())
		}
	}
	// Encoding fails closed when its output would not decode: values nested
	// past the payload depth cap, re-escaped past the byte cap, or holding
	// what the raw screen rejects, or outgrowing the caps once JSONB stores them.
	for _, value := range []string{
		strings.Repeat("[", 33) + strings.Repeat("]", 33), `"` + strings.Repeat("<", 100<<10) + `"`,
		`{"a":1,"a":2}`, `"\u0000"`, `1e2000`, `1e200`, "[" + strings.Repeat("1e120,", 4500) + "0]", "[" + strings.Repeat("0,", 200<<10) + "0]",
	} {
		_, err := EncodeControlPart(Control{RequestID: id, Kind: ControlCandidate, Value: json.RawMessage(value)})
		require.ErrorIs(t, err, ErrMalformedStructuredOutputMetadata, value[:min(len(value), 20)])
		_, err = EncodeOutcomePart(codersdk.ChatStructuredOutput{RequestID: id, Status: codersdk.ChatStructuredOutputStatusSucceeded, Value: json.RawMessage(value)})
		require.ErrorIs(t, err, ErrMalformedStructuredOutputMetadata, value[:min(len(value), 20)])
	}
	_, err = DecodeControlPart(part)
	require.ErrorIs(t, err, ErrMalformedStructuredOutputMetadata)
	for _, schema := range []json.RawMessage{nil, json.RawMessage(`{"maximum":1e200}`), json.RawMessage(`{"enum":[` + strings.Repeat("0,", 7000) + `0]}`)} {
		_, err = EncodeRequestPart(Request{RequestID: id, Name: "a", Schema: schema})
		require.ErrorIs(t, err, ErrMalformedStructuredOutputMetadata)
	}
	_, err = EncodeRequestPart(Request{RequestID: id, Name: "a", Description: "\xff", Schema: json.RawMessage(`{}`)})
	require.ErrorIs(t, err, ErrMalformedStructuredOutputMetadata)
	_, err = EncodeOutcomePart(codersdk.ChatStructuredOutput{RequestID: id, Status: "failed", Error: &codersdk.ChatStructuredOutputError{Code: "not_produced", Message: "\xff"}})
	require.ErrorIs(t, err, ErrMalformedStructuredOutputMetadata)
	_, err = EncodeRequestPart(Request{Name: "a", Schema: json.RawMessage(`{}`)})
	require.ErrorIs(t, err, ErrMalformedStructuredOutputMetadata)
}
