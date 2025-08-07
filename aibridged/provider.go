package aibridged

type Provider[Req any] interface {
	ParseRequest(payload []byte) (*Req, error)
	NewStreamingSession(*Req) Session
	NewBlockingSession(*Req) Session
}
