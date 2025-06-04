package aibridged

import (
	"fmt"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"strings"

	"golang.org/x/xerrors"
)

type Bridge struct {
	httpSrv *http.Server
	addr    string
}

func NewBridge(addr string) *Bridge {
	mux := &http.ServeMux{}
	mux.HandleFunc("/v1/chat/completions", proxyOpenAIRequest)
	mux.HandleFunc("/v1/messages", proxyAnthropicRequest)

	srv := &http.Server{
		Addr:    addr,
		Handler: mux,
		// TODO: other settings.
	}

	return &Bridge{httpSrv: srv}
}

func proxyOpenAIRequest(w http.ResponseWriter, r *http.Request) {
	target, err := url.Parse("https://api.openai.com")
	if err != nil {
		http.Error(w, "failed to parse OpenAI URL", http.StatusInternalServerError)
		return
	}

	proxy := httputil.NewSingleHostReverseProxy(target)
	originalDirector := proxy.Director
	proxy.Director = func(req *http.Request) {
		originalDirector(req)

		// Add OpenAI-specific headers
		if strings.TrimSpace(req.Header.Get("Authorization")) == "" {
			req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", os.Getenv("OPENAI_API_KEY")))
		}

		if req.Header.Get("Content-Type") == "" {
			req.Header.Set("Content-Type", "application/json")
		}

		req.Host = target.Host
		req.URL.Scheme = target.Scheme
		req.URL.Host = target.Host

		fmt.Printf("Proxying %s request to: %s\n", req.Method, req.URL.String())
	}
	proxy.ServeHTTP(w, r)
}

func proxyAnthropicRequest(w http.ResponseWriter, r *http.Request) {
	target, err := url.Parse("https://api.anthropic.com")
	if err != nil {
		http.Error(w, "failed to parse Anthropic URL", http.StatusInternalServerError)
		return
	}

	proxy := httputil.NewSingleHostReverseProxy(target)
	originalDirector := proxy.Director
	proxy.Director = func(req *http.Request) {
		originalDirector(req)

		// Add Anthropic-specific headers
		if strings.TrimSpace(req.Header.Get("x-api-key")) == "" {
			req.Header.Set("x-api-key", os.Getenv("ANTHROPIC_API_KEY"))
		}
		req.Header.Set("anthropic-version", "2023-06-01")

		if req.Header.Get("Content-Type") == "" {
			req.Header.Set("Content-Type", "application/json")
		}

		req.Host = target.Host
		req.URL.Scheme = target.Scheme
		req.URL.Host = target.Host

		fmt.Printf("Proxying %s request to: %s\n", req.Method, req.URL.String())
	}
	proxy.ServeHTTP(w, r)
}

func (b *Bridge) Serve() error {
	list, err := net.Listen("tcp", b.httpSrv.Addr)
	if err != nil {
		return xerrors.Errorf("listen: %w", err)
	}

	b.addr = list.Addr().String()

	return b.httpSrv.Serve(list) // TODO: TLS.
}

func (b *Bridge) Addr() string {
	return b.addr
}
