package fixtures

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws/protocol/eventstream"
	"github.com/stretchr/testify/require"
	"golang.org/x/tools/txtar"
)

// AWS EventStream classifies every wire frame (a "Message" in AWS
// terminology) with a two-level header hierarchy. The :message-type
// header discriminates the top level:
//
//	┌───────────────┬────────────────────────────────────────────────────────────────────┐
//	│ :message-type │ Meaning                                                            │
//	├───────────────┼────────────────────────────────────────────────────────────────────┤
//	│ event         │ A normal application-level event (the "happy path")                │
//	│ exception     │ A modeled application error (e.g., validation failure, throttling) │
//	│ error         │ A protocol-level error (malformed frame, internal server error)    │
//	└───────────────┴────────────────────────────────────────────────────────────────────┘
//
// A second header (:event-type, :exception-type, or :error-type)
// then names the sub-kind within that level. For Bedrock the only
// event sub-kind is "chunk" (a piece of generated content).
//
// We currently handle only :message-type=event + :event-type=chunk.
// Supporting exception events would require extending the parser and
// the encoder to handle the unwrapped exception payload.
const (
	headerMessageType = ":message-type"
	headerEventType   = ":event-type"

	messageTypeEvent = "event"
	eventTypeChunk   = "chunk"
)

// BedrockStreamFrame is one decoded EventStream frame. The inner
// Anthropic event JSON is decoded out of the {"bytes":"<base64>"}
// chunk envelope and stored in Event.
//
// Wire-to-struct mapping for one frame:
//
//	                  Wire bytes (one frame)                              Go: BedrockStreamFrame
//	┌─────────────────────────────────────────────────────────┐        ┌──────────────────────────┐
//	│ PRELUDE (12 bytes)                                      │        │                          │
//	│ ┌───────────────┬──────────────────┬──────────────────┐ │        │                          │
//	│ │ Total Length  │ Headers Length   │ Prelude CRC32    │ │        │                          │
//	│ │ (uint32 BE)   │ (uint32 BE)      │ (uint32 BE)      │ │ ─ ─ ─→ │  (validated, discarded)  │
//	│ │ 4 bytes       │ 4 bytes          │ 4 bytes          │ │        │                          │
//	│ └───────────────┴──────────────────┴──────────────────┘ │        │                          │
//	├─────────────────────────────────────────────────────────┤        ├──────────────────────────┤
//	│ HEADERS (variable, "Headers Length" bytes)              │        │ Headers map[string]string│
//	│ ┌─────────────────────────────────────────────────────┐ │        │                          │
//	│ │ name_len(1B) | name | type(1B) | val_len(2B) | val  │ │ ─────→ │  ":message-type": "event"│
//	│ │ name_len(1B) | name | type(1B) | val_len(2B) | val  │ │        │  ":event-type":   "chunk"│
//	│ │ name_len(1B) | name | type(1B) | val_len(2B) | val  │ │        │  ":content-type": "..."  │
//	│ │ ...                                                 │ │        │                          │
//	│ └─────────────────────────────────────────────────────┘ │        │                          │
//	├─────────────────────────────────────────────────────────┤        ├──────────────────────────┤
//	│ PAYLOAD (variable, raw bytes)                           │        │ Event json.RawMessage    │
//	│ ┌─────────────────────────────────────────────────────┐ │        │                          │
//	│ │ {"bytes":"<base64-of-inner-JSON>"}                  │ │        │  {"type":"message_start",│
//	│ │   ↓ base64-decode the "bytes" field                 │ │ ─────→ │   "message":{...}}       │
//	│ │ {"type":"message_start","message":{...}}            │ │        │  ↑ inner Anthropic event │
//	│ └─────────────────────────────────────────────────────┘ │        │    JSON, stored verbatim │
//	├─────────────────────────────────────────────────────────┤        ├──────────────────────────┤
//	│ MESSAGE CRC32 (4 bytes BE)                              │ ─ ─ ─→ │  (validated, discarded)  │
//	└─────────────────────────────────────────────────────────┘        └──────────────────────────┘
//
// Headers are kept verbatim (just type-flattened to strings) because
// the encoder needs them to rebuild the wire frame. Event is peeled
// one extra layer (chunk envelope + base64) so the on-disk fixture
// shows the actual semantic content rather than opaque base64.
type BedrockStreamFrame struct {
	Headers map[string]string `json:"headers"`
	Event   json.RawMessage   `json:"event"`
}

// BedrockFixture is the parsed shape of a Bedrock txtar fixture.
type BedrockFixture struct {
	t *testing.T

	// Request is the raw bytes of the "request" section.
	Request []byte
	// StreamingFrames is the parsed "streaming" section, one entry
	// per stream frame in order.
	StreamingFrames []BedrockStreamFrame
	// NonStreamingResponse is the raw bytes of the "non-streaming"
	// section.
	NonStreamingResponse []byte
}

// ParseBedrock parses a txtar fixture into a [BedrockFixture]. The
// fixture must contain at least one of "streaming" or "non-streaming";
// "request" is required.
func ParseBedrock(t *testing.T, data []byte) BedrockFixture {
	t.Helper()

	archive := txtar.Parse(data)
	require.NotEmpty(t, archive.Files, "bedrock fixture archive has no files")

	fix := BedrockFixture{t: t}
	for _, f := range archive.Files {
		switch f.Name {
		case fileRequest:
			fix.Request = f.Data
		case fileStreamingResponse:
			require.NoError(t, json.Unmarshal(f.Data, &fix.StreamingFrames),
				"unmarshal %q section", fileStreamingResponse)
			validateBedrockFrames(t, fix.StreamingFrames)
		case fileNonStreamingResponse:
			fix.NonStreamingResponse = f.Data
		default:
			t.Fatalf("bedrock fixture: unknown section %q", f.Name)
		}
	}
	require.NotEmpty(t, fix.Request, "bedrock fixture missing %q section", fileRequest)
	require.False(t, len(fix.StreamingFrames) == 0 && len(fix.NonStreamingResponse) == 0,
		"bedrock fixture must have %q or %q section", fileStreamingResponse, fileNonStreamingResponse)
	return fix
}

// validateBedrockFrames checks that each frame is a chunk event with
// valid JSON in its event field, the only variant we currently handle.
func validateBedrockFrames(t *testing.T, frames []BedrockStreamFrame) {
	t.Helper()
	for i, f := range frames {
		require.Equalf(t, messageTypeEvent, f.Headers[headerMessageType],
			"frame %d: only %q messages are supported, got %q",
			i, messageTypeEvent, f.Headers[headerMessageType])
		require.Equalf(t, eventTypeChunk, f.Headers[headerEventType],
			"frame %d: only %q events are supported, got %q",
			i, eventTypeChunk, f.Headers[headerEventType])
		require.Truef(t, json.Valid(f.Event),
			"frame %d: event field is not valid JSON", i)
	}
}

// EncodeAsAWSEventStream serializes StreamingFrames as the AWS
// EventStream wire format: each frame's Event is compacted,
// base64-encoded, and wrapped in {"bytes":"..."}.
//
// The resulting bytes are not byte-for-byte identical to an original
// AWS capture (we re-marshal JSON with Go's encoder), but the decoded
// content is semantically identical.
func (f BedrockFixture) EncodeAsAWSEventStream() []byte {
	f.t.Helper()
	var out bytes.Buffer
	enc := eventstream.NewEncoder()
	for i, frame := range f.StreamingFrames {
		var compact bytes.Buffer
		require.NoErrorf(f.t, json.Compact(&compact, frame.Event),
			"frame %d: compact event", i)

		b64 := base64.StdEncoding.EncodeToString(compact.Bytes())
		payload := []byte(`{"bytes":"` + b64 + `"}`)

		require.NoErrorf(f.t, enc.Encode(&out, eventstream.Message{
			Headers: convertStringHeaders(frame.Headers),
			Payload: payload,
		}), "frame %d: encode message", i)
	}
	return out.Bytes()
}

// DecodeAWSEventStream parses raw EventStream bytes (e.g. captured
// from a real Bedrock invoke-with-response-stream call) into the
// human-readable [BedrockStreamFrame] form. It is the inverse of
// [BedrockFixture.EncodeAsAWSEventStream] and is the workhorse of the
// bedrock-fixture CLI tool. Only chunk events are supported.
func DecodeAWSEventStream(data []byte) ([]BedrockStreamFrame, error) {
	dec := eventstream.NewDecoder()
	r := bytes.NewReader(data)
	var frames []BedrockStreamFrame
	for {
		msg, err := dec.Decode(r, nil)
		if err != nil {
			if err == io.EOF {
				return frames, nil
			}
			return nil, fmt.Errorf("decode message: %w", err)
		}

		headers := make(map[string]string, len(msg.Headers))
		for _, h := range msg.Headers {
			sv, ok := h.Value.(eventstream.StringValue)
			if !ok {
				return nil, fmt.Errorf("header %q: non-string value type %T", h.Name, h.Value)
			}
			headers[h.Name] = string(sv)
		}
		if mt := headers[headerMessageType]; mt != messageTypeEvent {
			return nil, fmt.Errorf("only %q messages are supported, got %q", messageTypeEvent, mt)
		}
		if et := headers[headerEventType]; et != eventTypeChunk {
			return nil, fmt.Errorf("only %q events are supported, got %q", eventTypeChunk, et)
		}

		event, err := unwrapChunkPayload(msg.Payload)
		if err != nil {
			return nil, fmt.Errorf("unwrap chunk payload: %w", err)
		}
		frames = append(frames, BedrockStreamFrame{Headers: headers, Event: event})
	}
}

// unwrapChunkPayload extracts the inner Anthropic event JSON from a
// {"bytes":"<base64>"} chunk envelope.
func unwrapChunkPayload(payload []byte) (json.RawMessage, error) {
	var envelope struct {
		Bytes []byte `json:"bytes"`
	}
	if err := json.Unmarshal(payload, &envelope); err != nil {
		return nil, fmt.Errorf("unmarshal chunk envelope: %w", err)
	}
	if !json.Valid(envelope.Bytes) {
		return nil, fmt.Errorf("decoded chunk bytes are not valid JSON")
	}
	return json.RawMessage(envelope.Bytes), nil
}

func convertStringHeaders(h map[string]string) eventstream.Headers {
	out := make(eventstream.Headers, 0, len(h))
	for name, value := range h {
		out = append(out, eventstream.Header{
			Name:  name,
			Value: eventstream.StringValue(value),
		})
	}
	return out
}
