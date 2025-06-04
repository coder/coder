package aibridged

import (
	"fmt"
	"net"
	"net/http"

	"golang.org/x/xerrors"
)

type Bridge struct {
	srv *http.Server
	addr string
}

func NewBridge(addr string) *Bridge {
	mux := &http.ServeMux{}
	mux.HandleFunc("/v1/chat/completions", bridgeOpenAIRequest)
	mux.HandleFunc("/v1/messages", bridgeAnthropicRequest)

	srv := &http.Server{
		Addr:    addr,
		Handler: mux,
		// TODO: other settings.
	}

	return &Bridge{srv: srv}
}

func (b *Bridge) Serve() error {
	list, err := net.Listen("tcp", b.srv.Addr)
	if err != nil {
		return xerrors.Errorf("listen: %w", err)
	}

	b.addr = list.Addr().String()

	return b.srv.Serve(list) // TODO: TLS.
}

func (b *Bridge) Addr() string {
	return b.addr
}

func bridgeOpenAIRequest(w http.ResponseWriter, r *http.Request) {
	fmt.Println("OpenAI")
}

func bridgeAnthropicRequest(w http.ResponseWriter, r *http.Request) {
	fmt.Println("Anthropic")
}

func bridgeRequest(w http.ResponseWriter, r *http.Request) {

}
