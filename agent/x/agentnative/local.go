package agentnative

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net"
	"net/http"
	"os"
	"strconv"
	"time"
)

type localServer struct {
	manager    *Manager
	descriptor Descriptor
	listener   net.Listener
	server     *http.Server
	handler    http.Handler
}

func newLocalServer(manager *Manager) (*localServer, error) {
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	local := &localServer{manager: manager, listener: listener, descriptor: Descriptor{URL: "http://" + listener.Addr().String(), Token: rand.Text() + rand.Text()}}
	local.handler = local.routes()
	local.server = &http.Server{Handler: local.handler, ReadHeaderTimeout: 5 * time.Second, IdleTimeout: 30 * time.Second, MaxHeaderBytes: 16384}
	return local, nil
}

func (l *localServer) start() error {
	data, err := json.Marshal(l.descriptor)
	if err != nil {
		return err
	}
	_, stamp, err := l.manager.store.dir.read("manager.json", 16384)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if _, err := l.manager.store.dir.write("manager.json", append(data, '\n'), stamp); err != nil {
		return err
	}
	go func() { _ = l.server.Serve(l.listener) }()
	return nil
}

func (l *localServer) close() error {
	// Close the listener explicitly as initialization can fail before Serve.
	err := l.server.Close()
	listenerErr := l.listener.Close()
	if errors.Is(listenerErr, net.ErrClosed) {
		listenerErr = nil
	}
	return errors.Join(err, listenerErr)
}

func (l *localServer) routes() http.Handler {
	m := l.manager
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/sessions", func(w http.ResponseWriter, _ *http.Request) { localJSON(w, map[string]any{"sessions": m.List()}, nil) })
	mux.HandleFunc("POST /v1/sessions", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Harness string `json:"harness"`
			Cwd     string `json:"cwd"`
		}
		if !localBody(w, r, &body) {
			return
		}
		s, err := m.Create(r.Context(), body.Harness, body.Cwd)
		localJSON(w, s, err)
	})
	mux.HandleFunc("GET /v1/sessions/{id}", func(w http.ResponseWriter, r *http.Request) {
		s, err := m.Session(r.PathValue("id"))
		localJSON(w, s, err)
	})
	mux.HandleFunc("GET /v1/sessions/{id}/events", func(w http.ResponseWriter, r *http.Request) {
		after := uint64(0)
		if value := r.URL.Query().Get("after"); value != "" {
			var err error
			after, err = strconv.ParseUint(value, 10, 64)
			if err != nil {
				http.Error(w, "invalid event cursor", http.StatusBadRequest)
				return
			}
		}
		events, cursor, err := m.Events(r.Context(), r.PathValue("id"), after, r.URL.Query().Get("wait") == "1")
		localJSON(w, map[string]any{"events": events, "cursor": cursor}, err)
	})
	mux.HandleFunc("POST /v1/sessions/{id}/mode", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Mode string `json:"mode"`
		}
		if !localBody(w, r, &body) {
			return
		}
		localJSON(w, struct{}{}, m.SetMode(r.Context(), r.PathValue("id"), body.Mode))
	})
	mux.HandleFunc("POST /v1/sessions/{id}/prompt", func(w http.ResponseWriter, r *http.Request) {
		var body Prompt
		if !localBody(w, r, &body) {
			return
		}
		localJSON(w, struct{}{}, m.Submit(r.Context(), r.PathValue("id"), body))
	})
	mux.HandleFunc("POST /v1/sessions/{id}/cancel", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			RequestID string `json:"requestId"`
		}
		if !localBody(w, r, &body) {
			return
		}
		localJSON(w, struct{}{}, m.Cancel(r.Context(), r.PathValue("id"), body.RequestID))
	})
	mux.HandleFunc("POST /v1/sessions/{id}/attach", func(w http.ResponseWriter, r *http.Request) {
		var body struct{}
		if !localBody(w, r, &body) {
			return
		}
		command, err := m.Attach(r.Context(), r.PathValue("id"))
		localJSON(w, command, err)
	})
	restore := func(w http.ResponseWriter, r *http.Request) {
		var body struct{}
		if !localBody(w, r, &body) {
			return
		}
		localJSON(w, struct{}{}, m.Restore(r.Context()))
	}
	shutdown := func(w http.ResponseWriter, r *http.Request) {
		var body struct{}
		if !localBody(w, r, &body) {
			return
		}
		localJSON(w, struct{}{}, m.Shutdown(r.Context()))
	}
	mux.HandleFunc("POST /v1/restore", restore)
	mux.HandleFunc("POST /v1/shutdown", shutdown)
	mux.HandleFunc("POST /restore", restore)
	mux.HandleFunc("POST /shutdown", shutdown)
	mux.HandleFunc("POST /v1/runtime", func(w http.ResponseWriter, r *http.Request) {
		handler, ok := m.backends["claude"].(http.Handler)
		if !ok {
			http.Error(w, "Claude callbacks unavailable", http.StatusServiceUnavailable)
			return
		}
		handler.ServeHTTP(w, r)
	})
	active := make(chan struct{}, 128)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		expected := "Bearer " + l.descriptor.Token
		if r.Header.Get("Origin") != "" || r.Host != l.listener.Addr().String() || subtle.ConstantTimeCompare([]byte(r.Header.Get("Authorization")), []byte(expected)) != 1 {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		select {
		case active <- struct{}{}:
			defer func() { <-active }()
		default:
			http.Error(w, "local request budget exhausted", http.StatusServiceUnavailable)
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, 4<<20)
		mux.ServeHTTP(w, r)
	})
}

func localBody(w http.ResponseWriter, r *http.Request, value any) bool {
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/json" {
		http.Error(w, "expected application/json", http.StatusBadRequest)
		return false
	}
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(value); err != nil {
		http.Error(w, "invalid JSON body", http.StatusBadRequest)
		return false
	}
	if err := decoder.Decode(new(any)); !errors.Is(err, io.EOF) {
		http.Error(w, "invalid JSON body", http.StatusBadRequest)
		return false
	}
	return true
}

func localJSON(w http.ResponseWriter, value any, err error) {
	w.Header().Set("Content-Type", "application/json")
	if err != nil {
		w.WriteHeader(http.StatusConflict)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	_ = json.NewEncoder(w).Encode(value)
}
