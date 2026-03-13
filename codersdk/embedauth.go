package codersdk

// EmbedSessionTokenRequest contains an existing session token to bootstrap a
// browser cookie for embedded chat access.
type EmbedSessionTokenRequest struct {
	Token string `json:"token"`
}
