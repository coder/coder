//go:build linux

package agentnative

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/quartz"
)

type managerHTTPBackend struct {
	managerTestBackend
	callbacks atomic.Int32
}

func (b *managerHTTPBackend) ServeHTTP(w http.ResponseWriter, _ *http.Request) {
	b.callbacks.Add(1)
	w.WriteHeader(http.StatusNoContent)
}

func TestManagerLocalAuthenticationAndRoutes(t *testing.T) {
	t.Parallel()
	dir := filepath.Join(t.TempDir(), "state")
	b := &managerHTTPBackend{}
	m, err := New(Options{StateDir: dir, Clock: quartz.NewMock(t), Factories: map[string]func(BackendOptions) (Backend, error){
		"claude": func(o BackendOptions) (Backend, error) { b.host = o.Host; return b, nil },
	}})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, m.Close()) })
	descriptor, err := ReadDescriptor(dir)
	require.NoError(t, err)
	endpoint, err := url.Parse(descriptor.URL)
	require.NoError(t, err)
	for _, test := range []struct {
		name, token, host, origin string
		want                      int
	}{
		{"missing", "", endpoint.Host, "", http.StatusForbidden},
		{"wrong-token", "wrong", endpoint.Host, "", http.StatusForbidden},
		{"wrong-host", descriptor.Token, "host.example", "", http.StatusForbidden},
		{"browser-origin", descriptor.Token, endpoint.Host, "https://example.test", http.StatusForbidden},
		{"authorized", descriptor.Token, endpoint.Host, "", http.StatusOK},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			request := httptest.NewRequest(http.MethodGet, "/v1/sessions", nil)
			request.Host = test.host
			request.Header.Set("Authorization", "Bearer "+test.token)
			request.Header.Set("Origin", test.origin)
			recorder := httptest.NewRecorder()
			m.Handler().ServeHTTP(recorder, request)
			require.Equal(t, test.want, recorder.Code)
			require.Equal(t, "no-store", recorder.Header().Get("Cache-Control"))
		})
	}
	request := httptest.NewRequest(http.MethodPost, "/v1/runtime", strings.NewReader(`{}`))
	request.Host = endpoint.Host
	request.Header.Set("Authorization", "Bearer "+descriptor.Token)
	recorder := httptest.NewRecorder()
	m.Handler().ServeHTTP(recorder, request)
	require.Equal(t, http.StatusNoContent, recorder.Code)
	require.EqualValues(t, 1, b.callbacks.Load())
	request.Header.Del("Authorization")
	recorder = httptest.NewRecorder()
	m.Handler().ServeHTTP(recorder, request)
	require.Equal(t, http.StatusForbidden, recorder.Code)
	require.EqualValues(t, 1, b.callbacks.Load())

	request, err = http.NewRequestWithContext(t.Context(), http.MethodGet, descriptor.URL+"/v1/sessions", nil)
	require.NoError(t, err)
	request.Header.Set("Authorization", "Bearer "+descriptor.Token)
	response, err := http.DefaultClient.Do(request)
	require.NoError(t, err)
	defer response.Body.Close()
	require.Equal(t, http.StatusOK, response.StatusCode)
	var result struct {
		Sessions []Session `json:"sessions"`
	}
	require.NoError(t, json.NewDecoder(response.Body).Decode(&result))
	require.Empty(t, result.Sessions)
}
