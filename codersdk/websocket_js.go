//go:build js

package codersdk

import (
	"net/http"

	"nhooyr.io/websocket"
)

func websocketOptions(_ *http.Client, _ websocket.CompressionMode) *websocket.DialOptions {
	return &websocket.DialOptions{}
}
