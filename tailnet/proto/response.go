package proto

// maxPeerUpdatesPerMessage is the maximum number of peer updates that
// can be sent in a single CoordinateResponse to stay under DRPC
// message size limits.
const maxPeerUpdatesPerMessage = 1024

// Chunked splits the response into multiple responses, each containing
// at most maxPeerUpdatesPerMessage peer updates to stay under the DRPC
// 4 MiB transport limit.
func (r *CoordinateResponse) Chunked() []*CoordinateResponse {
	updates := r.GetPeerUpdates()
	if len(updates) <= maxPeerUpdatesPerMessage {
		return []*CoordinateResponse{r}
	}
	var chunks []*CoordinateResponse
	for i := 0; i < len(updates); i += maxPeerUpdatesPerMessage {
		end := min(i+maxPeerUpdatesPerMessage, len(updates))
		chunks = append(chunks, &CoordinateResponse{PeerUpdates: updates[i:end]})
	}
	return chunks
}
