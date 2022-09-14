package codersdk

import (
	"bufio"
	"fmt"
	"io"
	"strings"

	"golang.org/x/xerrors"
)

type ServerSideEvent struct {
	Type EventType
	Data interface{}
}

type EventType string

const (
	EventTypePing  EventType = "ping"
	EventTypeData  EventType = "data"
	EventTypeError EventType = "error"
)

func ServerSideEventReader(rc io.ReadCloser) func() (*ServerSideEvent, error) {
	reader := bufio.NewReader(rc)
	nextLineValue := func(prefix string) ([]byte, error) {
		var line string
		var err error
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
	return func() (*ServerSideEvent, error) {
		for {
			t, err := nextLineValue("event")
			if err != nil {
				return nil, xerrors.Errorf("reading next line value: %w", err)
			}

			switch EventType(t) {
			case EventTypePing:
				return &ServerSideEvent{
					Type: EventTypePing,
				}, nil
			case EventTypeData:
				d, err := nextLineValue("data")
				if err != nil {
					return nil, xerrors.Errorf("reading next line value: %w", err)
				}

				return &ServerSideEvent{
					Type: EventTypeData,
					Data: d,
				}, nil
			case EventTypeError:
				d, err := nextLineValue("data")
				if err != nil {
					return nil, xerrors.Errorf("reading next line value: %w", err)
				}

				return &ServerSideEvent{
					Type: EventTypeError,
					Data: d,
				}, nil
			default:
				return nil, xerrors.Errorf("unknown event type: %s", t)
			}
		}
	}
}
