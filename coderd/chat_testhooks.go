package coderd

import "github.com/coder/coder/v2/coderd/x/chatd"

// ChatDaemonForTest returns the background chat processor for test harnesses.
func (api *API) ChatDaemonForTest() *chatd.Server {
	return api.chatDaemon
}
