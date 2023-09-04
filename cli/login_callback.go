package cli

import (
	"fmt"
	"net"
	"net/http"
	"net/url"
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
		h.Server.Stop()
	}
}

func NewServer(authURL url.URL, addr string, handler http.Handler) *Server {
	server := &Server{
		authURL: &authURL,
		httpServer: &http.Server{
			Addr:    addr,
			Handler: handler,
		},
		resultChan: make(chan TokenResponse),
		stopChan:   make(chan struct{}),
	}

	return server
}

func (s *Server) Start() {
	go func() {
		fmt.Printf("HTTP server listening on %s\n", s.httpServer.Addr)
		if err := s.httpServer.ListenAndServe(); err != http.ErrServerClosed {
			s.resultChan <- TokenResponse{token: "", err: err}
		}
	}()
}

func (s *Server) Stop() {
	s.httpServer.Close()
	close(s.stopChan)
}

func (s *Server) Wait() {
	<-s.stopChan
	fmt.Println("HTTP server stopped.")
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

	server.authURL.Query().Add("callback", server.GenerateCallbackPath())

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
	return l.Addr().(*net.TCPAddr).Port, nil
}

func (s *Server) GenerateCallbackPath() string {
	return "http://" + s.httpServer.Addr + "/auth"
}
