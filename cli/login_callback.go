package cli

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"time"
)

type TokenResponse struct {
	token string
	err   error
}

type Server struct {
	authURL    *url.URL
	httpServer *http.Server
	resultChan chan TokenResponse
	stopChan   chan struct{}
}

type HandlerWithContext struct {
	*Server
}

func (h *HandlerWithContext) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/auth" {
		token := r.URL.Query().Get("token")
		h.resultChan <- TokenResponse{token: token, err: nil}
		_, _ = fmt.Fprintf(w, "<html><head><script><title>Token received!</title></head><body><a href='#' onclick='javascript:window.close();'>Close this Window</a></body></html>")

		h.Server.Stop()
	}
}

func NewServer(authURL url.URL, addr string, handler http.Handler) *Server {
	server := &Server{
		authURL: &authURL,
		httpServer: &http.Server{
			Addr:    addr,
			Handler: handler,

			ReadTimeout:       1 * time.Second,
			WriteTimeout:      1 * time.Second,
			IdleTimeout:       30 * time.Second,
			ReadHeaderTimeout: 2 * time.Second,
		},
		resultChan: make(chan TokenResponse),
		stopChan:   make(chan struct{}),
	}

	return server
}

func (s *Server) Start() {
	go func() {
		_, _ = fmt.Printf("HTTP server listening on %s\n", s.httpServer.Addr)
		if err := s.httpServer.ListenAndServe(); err != http.ErrServerClosed {
			s.resultChan <- TokenResponse{token: "", err: err}
		}
	}()
}

func (s *Server) Stop() {
	err := s.httpServer.Close()
	if err != nil {
		_, _ = fmt.Printf("Unable to stop HTTP Server")
	}

	close(s.stopChan)
}

func (s *Server) Wait() {
	<-s.stopChan
	_, _ = fmt.Println("HTTP server stopped.")
}

func getListenAddr(port int) string {
	return fmt.Sprintf("127.0.0.1:%d", port)
}

func getServer(authURL url.URL) (*Server, error) {
	port, err := getAvailablePort()
	if err != nil {
		return &Server{}, err
	}

	server := NewServer(authURL, getListenAddr((port)), &HandlerWithContext{})

	server.Start()

	values := server.authURL.Query()
	values.Add("callback", server.GenerateCallbackPath())

	server.authURL.RawQuery = values.Encode()

	return server, nil
}

func getAvailablePort() (int, error) {
	addr, err := net.ResolveTCPAddr("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}

	l, err := net.ListenTCP("tcp", addr)
	if err != nil {
		return 0, err
	}

	defer l.Close()

	if tcpAddr, ok := l.Addr().(*net.TCPAddr); ok {
		port := tcpAddr.Port
		return port, nil
	} else {
		return -1, errors.New("Unable to get an available port")
	}
}

func (s *Server) GenerateCallbackPath() string {
	return "http://" + s.httpServer.Addr + "/auth"
}
