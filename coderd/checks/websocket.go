package checks

import (
	"context"
	"net/url"
	"path/filepath"
	"time"

	"golang.org/x/xerrors"
	"nhooyr.io/websocket"
)

func CanDialWebsocket(accessURL *url.URL, timeout time.Duration) CheckFunc {
	wsURL := *accessURL
	switch accessURL.Scheme {
	case "http":
		wsURL.Scheme = "ws"
	case "https":
		wsURL.Scheme = "wss"
	default:
	}
	wsURL.Path = filepath.Join(wsURL.Path, "checks", "websocket")
	return func() error {
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()
		c, _, err := websocket.Dial(ctx, wsURL.String(), &websocket.DialOptions{})
		if err != nil {
			return xerrors.Errorf("websocket dial %q: %w", wsURL.String(), err)
		}
		msgType, _, err := c.Read(ctx)
		if err != nil {
			return xerrors.Errorf("read websocket msg: %w", err)
		}
		if msgType != websocket.MessageText {
			return xerrors.Errorf("unexpected websocket msg type: %q", msgType)
		}
		return nil
	}
}
