//go:build !js

package codersdk

import (
	"net/http"

	"nhooyr.io/websocket"
)

func websocketOptions(httpClient *http.Client, compression websocket.CompressionMode) *websocket.DialOptions {
	return &websocket.DialOptions{
		HTTPClient:      httpClient,
		CompressionMode: compression,
	}
}
