package messages //nolint:testpackage // tests unexported internals

import (
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws/protocol/eventstream"
	"github.com/aws/aws-sdk-go-v2/aws/protocol/eventstream/eventstreamapi"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/aibridge/config"
	"github.com/coder/coder/v2/aibridge/intercept"
	"github.com/coder/coder/v2/aibridge/internal/testutil"
)

// bedrockChunkEvents is the SSE event payload Bedrock relays inside
// AWS event-stream "chunk" messages. The sequence mirrors a normal,
// fully successful Anthropic streaming response.
var bedrockChunkEvents = []string{
	`{"type":"message_start","message":{"id":"msg_01","type":"message","role":"assistant","model":"claude-opus-4-5","content":[],"stop_reason":null,"stop_sequence":null,"usage":{"input_tokens":10,"output_tokens":1}}}`,
	`{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`,
	`{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"hi"}}`,
	`{"type":"content_block_stop","index":0}`,
	`{"type":"message_delta","delta":{"stop_reason":"end_turn","stop_sequence":null},"usage":{"output_tokens":5}}`,
	`{"type":"message_stop"}`,
}

// TestStreamingInterception_BedrockCleanEOF reproduces the failure
// where a fully successful Bedrock streaming response was reported
// back to the caller as an interception error of "EOF".
//
// The Bedrock event-stream decoder
// ([bedrock.eventstreamDecoder.Next]) captures io.EOF from the
// underlying AWS event-stream reader when the upstream body is
// closed cleanly after the final chunk. ProcessRequest must
// treat that signal as a successful completion and return nil.
func TestStreamingInterception_BedrockCleanEOF(t *testing.T) {
	t.Parallel()

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		w.Header().Set("Content-Type", "application/vnd.amazon.eventstream")
		w.WriteHeader(http.StatusOK)

		enc := eventstream.NewEncoder()
		for _, ev := range bedrockChunkEvents {
			payloadJSON, err := json.Marshal(struct {
				Bytes string `json:"bytes"`
				P     string `json:"p"`
			}{
				Bytes: base64.StdEncoding.EncodeToString([]byte(ev)),
				P:     "abcd",
			})
			if err != nil {
				t.Errorf("marshal chunk payload: %v", err)
				return
			}
			msg := eventstream.Message{
				Payload: payloadJSON,
			}
			msg.Headers.Set(eventstreamapi.MessageTypeHeader, eventstream.StringValue(eventstreamapi.EventMessageType))
			msg.Headers.Set(eventstreamapi.EventTypeHeader, eventstream.StringValue("chunk"))
			msg.Headers.Set(eventstreamapi.ContentTypeHeader, eventstream.StringValue("application/json"))
			if err := enc.Encode(w, msg); err != nil {
				t.Errorf("encode chunk: %v", err)
				return
			}
		}
		// Body returns naturally here; the SDK reader hits EOF on
		// its next Decode call, which is the condition under test.
	}))
	t.Cleanup(upstream.Close)

	payload, err := NewRequestPayload([]byte(requestBody))
	require.NoError(t, err)

	bedrockCfg := &config.AWSBedrock{
		Region:          "us-east-1",
		BaseURL:         upstream.URL,
		Model:           "anthropic.claude-opus-4-5-v1:0",
		SmallFastModel:  "anthropic.claude-haiku-4-5-v1:0",
		AccessKey:       "AKIATEST",
		AccessKeySecret: "secret",
	}

	interceptor := NewStreamingInterceptor(
		uuid.New(),
		payload,
		config.ProviderAnthropic,
		config.Anthropic{},
		bedrockCfg,
		http.Header{},
		"X-Api-Key",
		otel.Tracer("streaming_bedrock_eof_test"),
		intercept.NewCredentialInfo(intercept.CredentialKindCentralized, ""),
	)
	interceptor.Setup(slog.Make(), &testutil.MockRecorder{}, nil)

	req := httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
	w := httptest.NewRecorder()
	require.NoError(t, interceptor.ProcessRequest(w, req),
		"clean Bedrock stream completion must not be reported as an interception error")
	require.Equal(t, http.StatusOK, w.Code)
}
