package codersdk

import (
	"bufio"
	"fmt"
	"io"
	"strings"

	"golang.org/x/xerrors"
)

type ServerSideEvent struct {
	Type ServerSideEventType
	Data interface{}
}

type ServerSideEventType string

const (
	ServerSideEventTypePing  ServerSideEventType = "ping"
	ServerSideEventTypeData  ServerSideEventType = "data"
	ServerSideEventTypeError ServerSideEventType = "error"
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

			switch ServerSideEventType(t) {
			case ServerSideEventTypePing:
				return &ServerSideEvent{
					Type: ServerSideEventTypePing,
				}, nil
			case ServerSideEventTypeData:
				d, err := nextLineValue("data")
				if err != nil {
					return nil, xerrors.Errorf("reading next line value: %w", err)
				}

				return &ServerSideEvent{
					Type: ServerSideEventTypeData,
					Data: d,
				}, nil
			case ServerSideEventTypeError:
				d, err := nextLineValue("data")
				if err != nil {
					return nil, xerrors.Errorf("reading next line value: %w", err)
				}

				return &ServerSideEvent{
					Type: ServerSideEventTypeError,
					Data: d,
				}, nil
			default:
				return nil, xerrors.Errorf("unknown event type: %s", t)
			}
		}
	}
}
