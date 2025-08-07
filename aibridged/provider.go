package aibridged

type Provider[Req any] interface {
	ParseRequest(payload []byte) (*Req, error)
	NewAsynchronousSession(*Req) Session[Req]
	NewSynchronousSession(*Req) Session[Req]
}
