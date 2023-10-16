package codersdk

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"strings"

	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/tracing"
)

type ServerSentEvent struct {
	Type ServerSentEventType `json:"type"`
	Data interface{}         `json:"data"`
}

type ServerSentEventType string

const (
	ServerSentEventTypePing  ServerSentEventType = "ping"
	ServerSentEventTypeData  ServerSentEventType = "data"
	ServerSentEventTypeError ServerSentEventType = "error"
)

func ServerSentEventReader(ctx context.Context, rc io.ReadCloser) func() (*ServerSentEvent, error) {
	_, span := tracing.StartSpan(ctx)
	defer span.End()

	reader := bufio.NewReader(rc)
	nextLineValue := func(prefix string) ([]byte, error) {
		var (
			line string
			err  error
		)
		for {
			line, err = reader.ReadString('\n')
			if err != nil {
				return nil, xerrors.Errorf("reading next string: %w", err)
			}
			if strings.TrimSpace(line) != "" {
				break
			}
		}

		if !strings.HasPrefix(line, fmt.Sprintf("%s: ", prefix)) {
			return nil, xerrors.Errorf("expecting %s prefix, got: %s", prefix, line)
		}
		s := strings.TrimPrefix(line, fmt.Sprintf("%s: ", prefix))
		s = strings.TrimSpace(s)
		return []byte(s), nil
	}

	nextEvent := func() (*ServerSentEvent, error) {
		for {
			t, err := nextLineValue("event")
			if err != nil {
				return nil, xerrors.Errorf("reading next line value: %w", err)
			}

			switch ServerSentEventType(t) {
			case ServerSentEventTypePing:
				return &ServerSentEvent{
					Type: ServerSentEventTypePing,
				}, nil
			case ServerSentEventTypeData:
				d, err := nextLineValue("data")
				if err != nil {
					return nil, xerrors.Errorf("reading next line value: %w", err)
				}

				return &ServerSentEvent{
					Type: ServerSentEventTypeData,
					Data: d,
				}, nil
			case ServerSentEventTypeError:
				d, err := nextLineValue("data")
				if err != nil {
					return nil, xerrors.Errorf("reading next line value: %w", err)
				}

				return &ServerSentEvent{
					Type: ServerSentEventTypeError,
					Data: d,
				}, nil
			default:
				return nil, xerrors.Errorf("unknown event type: %s", t)
			}
		}
	}

	return nextEvent
}
