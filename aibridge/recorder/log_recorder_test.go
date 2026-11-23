package recorder_test

import (
	"bufio"
	"bytes"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3"
	"cdr.dev/slog/v3/sloggers/slogjson"
	"github.com/coder/coder/v2/aibridge/recorder"
)

func TestLogRecorderStructuredTokenUsage(t *testing.T) {
	t.Parallel()

	buf := &bytes.Buffer{}
	rec := recorder.NewLogRecorder(slog.Make(slogjson.Sink(buf)), true, nil)
	err := rec.RecordTokenUsage(t.Context(), &recorder.TokenUsageRecord{
		InterceptionID:          "interception-1",
		MsgID:                   "msg-1",
		Output:                  200,
		CacheWriteInputTokens:   50,
		CacheWrite1hInputTokens: 30,
		PromptTokens:            400,
	})
	require.NoError(t, err)

	var fields map[string]any
	scanner := bufio.NewScanner(buf)
	for scanner.Scan() {
		var line struct {
			Msg    string         `json:"msg"`
			Fields map[string]any `json:"fields"`
		}
		require.NoError(t, json.Unmarshal(scanner.Bytes(), &line))
		if line.Msg == recorder.InterceptionLogMarker {
			fields = line.Fields
		}
	}
	require.NotNil(t, fields, "no structured record was logged")
	require.Equal(t, recorder.RecordTypeTokenUsage, fields["record_type"])
	require.Equal(t, float64(200), fields["output_tokens"]) // JSON numbers are float64.
	require.Equal(t, float64(30), fields["cache_write_one_hour_input_tokens"])
	require.Equal(t, float64(400), fields["prompt_tokens"])
}
