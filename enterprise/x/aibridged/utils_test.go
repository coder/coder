package aibridged_test

import (
	"net/http"
	"sync/atomic"
)

var _ http.Handler = &mockAIUpstreamServer{}

type mockAIUpstreamServer struct {
	hitCounter atomic.Int32
}

func (m *mockAIUpstreamServer) ServeHTTP(rw http.ResponseWriter, _ *http.Request) {
	m.hitCounter.Add(1)

	rw.Header().Set("Content-Type", "application/json")
	rw.WriteHeader(http.StatusOK)
	_, _ = rw.Write([]byte(`{}`))
}

func (m *mockAIUpstreamServer) Hits() int32 {
	return m.hitCounter.Load()
}
