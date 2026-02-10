package codersdk

// WorkspaceSessionsResponse is the response for listing workspace sessions.
type WorkspaceSessionsResponse struct {
	Sessions []WorkspaceSession `json:"sessions"`
	Count    int64              `json:"count"`
}
