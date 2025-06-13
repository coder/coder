package aibridged

import (
	"net/http"

	"cdr.dev/slog"
)

// BasicSSESender was implemented to overcome httpapi.ServerSentEventSender's odd design choices. For example, it doesn't
// write "event: data" for every data event (it's unnecessary, and breaks some AI tools' parsing of the SSE stream).
func BasicSSESender(eventsChan <-chan string, logger slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.Header().Set("X-Accel-Buffering", "no")

		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "SSE not supported", http.StatusInternalServerError)
			return
		}

		// Send initial flush to ensure connection is established.
		flusher.Flush()

		for {
			select {
			case <-ctx.Done():
				return
			case event, ok := <-eventsChan:
				if !ok {
					// Channel closed, send done event and exit
					_, err := w.Write([]byte("data: [DONE]\n\n")) // Convention used by OpenAI. // TODO: others, too?
					if err != nil {
						logger.Error(ctx, "failed to write done event", slog.Error(err))
					}
					flusher.Flush()
					return
				}

				// Send data event
				_, err := w.Write([]byte("data: " + event + "\n\n"))
				if err != nil {
					logger.Error(ctx, "failed to write SSE event", slog.Error(err))
					return
				}
				flusher.Flush()
			}
		}
	}
}
