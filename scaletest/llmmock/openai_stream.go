package llmmock

import (
	"context"
	"encoding/json"
	"io"
	"net/http"

	"cdr.dev/slog/v3"
)

type openAIToolCallDelta struct {
	Index    int                    `json:"index"`
	ID       string                 `json:"id,omitempty"`
	Type     string                 `json:"type,omitempty"`
	Function openAIToolCallFunction `json:"function"`
}

type openAIStreamDelta struct {
	Role      string                `json:"role,omitempty"`
	Content   *string               `json:"content,omitempty"`
	ToolCalls []openAIToolCallDelta `json:"tool_calls,omitempty"`
}

type openAIStreamChoice struct {
	Index        int               `json:"index"`
	Delta        openAIStreamDelta `json:"delta"`
	FinishReason *string           `json:"finish_reason"`
}

type openAIStreamChunk struct {
	ID      string               `json:"id"`
	Object  string               `json:"object"`
	Created int64                `json:"created"`
	Model   string               `json:"model"`
	Choices []openAIStreamChoice `json:"choices"`
}

type openAIStreamWriter struct {
	logger   slog.Logger
	w        http.ResponseWriter
	flusher  http.Flusher
	response openAIResponse
}

func (s *Server) newOpenAIStreamWriter(ctx context.Context, w http.ResponseWriter, resp openAIResponse) (openAIStreamWriter, bool) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		s.logger.Error(ctx, "responseWriter does not support flushing",
			slog.F("response_id", resp.ID),
		)
		return openAIStreamWriter{}, false
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)

	return openAIStreamWriter{
		logger:   s.logger,
		w:        w,
		flusher:  flusher,
		response: resp,
	}, true
}

func (sw openAIStreamWriter) writeDelta(ctx context.Context, delta openAIStreamDelta, finishReason *string) bool {
	chunk := openAIStreamChunk{
		ID:      sw.response.ID,
		Object:  "chat.completion.chunk",
		Created: sw.response.Created,
		Model:   sw.response.Model,
		Choices: []openAIStreamChoice{{
			Index:        0,
			Delta:        delta,
			FinishReason: finishReason,
		}},
	}
	data, _ := json.Marshal(chunk)
	return sw.writeData(ctx, data)
}

func (sw openAIStreamWriter) writeDone(ctx context.Context) bool {
	return sw.writeData(ctx, []byte("[DONE]"))
}

func (sw openAIStreamWriter) writeData(ctx context.Context, data []byte) bool {
	if _, err := io.WriteString(sw.w, "data: "); err != nil {
		return sw.logWriteError(ctx, err)
	}
	if _, err := sw.w.Write(data); err != nil {
		return sw.logWriteError(ctx, err)
	}
	if _, err := io.WriteString(sw.w, "\n\n"); err != nil {
		return sw.logWriteError(ctx, err)
	}
	sw.flusher.Flush()
	return true
}

func (sw openAIStreamWriter) logWriteError(ctx context.Context, err error) bool {
	sw.logger.Error(ctx, "failed to write OpenAI stream chunk",
		slog.F("response_id", sw.response.ID),
		slog.Error(err),
		slog.F("error_type", "write_error"),
		slog.F("likely_cause", "network_error"),
	)
	return false
}

func (s *Server) sendOpenAIStream(ctx context.Context, w http.ResponseWriter, resp openAIResponse) {
	writer, ok := s.newOpenAIStreamWriter(ctx, w, resp)
	if !ok {
		return
	}

	choice := resp.Choices[0]
	if len(choice.Message.ToolCalls) > 0 {
		if !writer.writeDelta(ctx, openAIStreamDelta{
			Role:      "assistant",
			ToolCalls: openAIStreamToolCallDeltas(choice.Message.ToolCalls),
		}, nil) {
			return
		}
	} else if !s.writeOpenAITextStream(ctx, writer, choice.Message.Content) {
		return
	}

	if !writer.writeDelta(ctx, openAIStreamDelta{}, &choice.FinishReason) {
		return
	}
	_ = writer.writeDone(ctx)
}

func (s *Server) writeOpenAITextStream(ctx context.Context, writer openAIStreamWriter, content string) bool {
	first := true
	for chunk := range s.streamContentChunks(ctx, s.randomStreamDuration(), content) {
		delta := openAITextDelta("", chunk)
		if first {
			delta.Role = "assistant"
			first = false
		}
		if !writer.writeDelta(ctx, delta, nil) {
			return false
		}
	}
	return ctx.Err() == nil
}

func openAITextDelta(role string, content string) openAIStreamDelta {
	return openAIStreamDelta{
		Role:    role,
		Content: &content,
	}
}

func openAIStreamToolCallDeltas(toolCalls []openAIToolCall) []openAIToolCallDelta {
	deltas := make([]openAIToolCallDelta, 0, len(toolCalls))
	for i, toolCall := range toolCalls {
		deltas = append(deltas, openAIToolCallDelta{
			Index:    i,
			ID:       toolCall.ID,
			Type:     toolCall.Type,
			Function: toolCall.Function,
		})
	}
	return deltas
}
