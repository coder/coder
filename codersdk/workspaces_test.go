package codersdk_test

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync/atomic"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/codersdk"
)

func TestResolveWorkspace(t *testing.T) {
	t.Parallel()

	// writeJSON is a small helper that writes a JSON-encoded value with
	// the given status code.
	writeJSON := func(w http.ResponseWriter, status int, v any) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(v)
	}

	// errResponse builds a codersdk.Response suitable for error replies.
	errResponse := func(msg string) codersdk.Response {
		return codersdk.Response{Message: msg}
	}

	t.Run("ByUUID", func(t *testing.T) {
		t.Parallel()

		wsID := uuid.New()
		expected := codersdk.Workspace{
			ID:   wsID,
			Name: "ws-by-uuid",
		}

		var uuidHits atomic.Int64

		r := chi.NewRouter()
		r.Get("/api/v2/workspaces/{workspace}", func(w http.ResponseWriter, req *http.Request) {
			uuidHits.Add(1)
			writeJSON(w, http.StatusOK, expected)
		})

		srv := httptest.NewServer(r)
		defer srv.Close()

		u, err := url.Parse(srv.URL)
		require.NoError(t, err)
		client := codersdk.New(u)

		ws, err := client.ResolveWorkspace(t.Context(), wsID.String())
		require.NoError(t, err)
		require.Equal(t, expected.ID, ws.ID)
		require.Equal(t, expected.Name, ws.Name)
		require.EqualValues(t, 1, uuidHits.Load(), "UUID endpoint should have been called")
	})

	t.Run("ByName", func(t *testing.T) {
		t.Parallel()

		expected := codersdk.Workspace{
			ID:   uuid.New(),
			Name: "my-workspace",
		}

		r := chi.NewRouter()
		r.Get("/api/v2/users/{user}/workspace/{workspace}", func(w http.ResponseWriter, req *http.Request) {
			owner := chi.URLParam(req, "user")
			name := chi.URLParam(req, "workspace")
			assert.Equal(t, "me", owner)
			assert.Equal(t, "my-workspace", name)
			writeJSON(w, http.StatusOK, expected)
		})

		srv := httptest.NewServer(r)
		defer srv.Close()

		u, err := url.Parse(srv.URL)
		require.NoError(t, err)
		client := codersdk.New(u)

		ws, err := client.ResolveWorkspace(t.Context(), "my-workspace")
		require.NoError(t, err)
		require.Equal(t, expected.ID, ws.ID)
		require.Equal(t, expected.Name, ws.Name)
	})

	t.Run("ByOwnerAndName", func(t *testing.T) {
		t.Parallel()

		expected := codersdk.Workspace{
			ID:   uuid.New(),
			Name: "my-workspace",
		}

		r := chi.NewRouter()
		r.Get("/api/v2/users/{user}/workspace/{workspace}", func(w http.ResponseWriter, req *http.Request) {
			owner := chi.URLParam(req, "user")
			name := chi.URLParam(req, "workspace")
			assert.Equal(t, "alice", owner)
			assert.Equal(t, "my-workspace", name)
			writeJSON(w, http.StatusOK, expected)
		})

		srv := httptest.NewServer(r)
		defer srv.Close()

		u, err := url.Parse(srv.URL)
		require.NoError(t, err)
		client := codersdk.New(u)

		ws, err := client.ResolveWorkspace(t.Context(), "alice/my-workspace")
		require.NoError(t, err)
		require.Equal(t, expected.ID, ws.ID)
		require.Equal(t, expected.Name, ws.Name)
	})

	t.Run("UUIDLikeNameFallback", func(t *testing.T) {
		t.Parallel()

		// 32 hex chars, a dashless UUID that is also a valid workspace
		// name (≤32 alphanumeric characters).
		const identifier = "a1b2c3d4e5f6a7b8c9d0e1f2a3b4c5d6"
		uuid.MustParse(identifier)

		expected := codersdk.Workspace{
			ID:   uuid.New(),
			Name: identifier,
		}

		var uuidHits, nameHits atomic.Int64

		r := chi.NewRouter()
		r.Get("/api/v2/workspaces/{workspace}", func(w http.ResponseWriter, req *http.Request) {
			uuidHits.Add(1)
			writeJSON(w, http.StatusNotFound, errResponse("Resource not found."))
		})
		r.Get("/api/v2/users/{user}/workspace/{workspace}", func(w http.ResponseWriter, req *http.Request) {
			nameHits.Add(1)
			writeJSON(w, http.StatusOK, expected)
		})

		srv := httptest.NewServer(r)
		defer srv.Close()

		u, err := url.Parse(srv.URL)
		require.NoError(t, err)
		client := codersdk.New(u)

		ws, err := client.ResolveWorkspace(t.Context(), identifier)
		require.NoError(t, err)
		require.Equal(t, expected.ID, ws.ID)
		require.EqualValues(t, 1, uuidHits.Load(), "UUID endpoint should have been tried first")
		require.EqualValues(t, 1, nameHits.Load(), "name endpoint should have been called as fallback")
	})

	t.Run("UUIDNotFoundNoName", func(t *testing.T) {
		t.Parallel()

		wsID := uuid.New()
		var uuidHits, nameHits atomic.Int64

		r := chi.NewRouter()
		r.Get("/api/v2/workspaces/{workspace}", func(w http.ResponseWriter, req *http.Request) {
			uuidHits.Add(1)
			writeJSON(w, http.StatusNotFound, errResponse("Resource not found."))
		})
		r.Get("/api/v2/users/{user}/workspace/{workspace}", func(w http.ResponseWriter, req *http.Request) {
			nameHits.Add(1)
			writeJSON(w, http.StatusNotFound, errResponse("Resource not found."))
		})

		srv := httptest.NewServer(r)
		defer srv.Close()

		u, err := url.Parse(srv.URL)
		require.NoError(t, err)
		client := codersdk.New(u)

		_, err = client.ResolveWorkspace(t.Context(), wsID.String())
		require.Error(t, err)

		var sdkErr *codersdk.Error
		require.ErrorAs(t, err, &sdkErr)
		require.Equal(t, http.StatusNotFound, sdkErr.StatusCode())
		require.EqualValues(t, 1, uuidHits.Load(), "UUID endpoint should have been called")
		require.EqualValues(t, 0, nameHits.Load(), "dashed UUID should not fall back to name lookup")
	})

	t.Run("NonNotFoundError", func(t *testing.T) {
		t.Parallel()

		wsID := uuid.New()

		var uuidHits, nameHits atomic.Int64

		r := chi.NewRouter()
		r.Get("/api/v2/workspaces/{workspace}", func(w http.ResponseWriter, req *http.Request) {
			uuidHits.Add(1)
			writeJSON(w, http.StatusInternalServerError, errResponse("Internal server error."))
		})
		r.Get("/api/v2/users/{user}/workspace/{workspace}", func(w http.ResponseWriter, req *http.Request) {
			nameHits.Add(1)
			writeJSON(w, http.StatusOK, codersdk.Workspace{})
		})

		srv := httptest.NewServer(r)
		defer srv.Close()

		u, err := url.Parse(srv.URL)
		require.NoError(t, err)
		client := codersdk.New(u)

		_, err = client.ResolveWorkspace(t.Context(), wsID.String())
		require.Error(t, err)

		var sdkErr *codersdk.Error
		require.ErrorAs(t, err, &sdkErr)
		require.Equal(t, http.StatusInternalServerError, sdkErr.StatusCode())
		require.EqualValues(t, 1, uuidHits.Load())
		require.EqualValues(t, 0, nameHits.Load(), "should not fall back on non-404 errors")
	})

	t.Run("TransportError", func(t *testing.T) {
		t.Parallel()

		// Close the server immediately so the transport layer fails.
		srv := httptest.NewServer(http.NotFoundHandler())
		srvURL, err := url.Parse(srv.URL)
		require.NoError(t, err)
		srv.Close()

		client := codersdk.New(srvURL)

		wsID := uuid.New()
		_, err = client.ResolveWorkspace(t.Context(), wsID.String())
		require.Error(t, err)

		// Transport errors must not be swallowed by the 404
		// fallback path. The error should NOT be a *codersdk.Error.
		var sdkErr *codersdk.Error
		require.False(t, errors.As(err, &sdkErr), "transport error should not be a codersdk.Error")
	})

	t.Run("InvalidIdentifier", func(t *testing.T) {
		t.Parallel()

		var hits atomic.Int64
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			hits.Add(1)
			t.Errorf("unexpected HTTP request for invalid identifier: %s", req.URL.Path)
		}))
		defer srv.Close()

		u, err := url.Parse(srv.URL)
		require.NoError(t, err)
		client := codersdk.New(u)

		_, err = client.ResolveWorkspace(t.Context(), "a/b/c")
		require.Error(t, err)
		require.ErrorContains(t, err, "invalid workspace identifier: \"a/b/c\"")
		require.EqualValues(t, 0, hits.Load(), "invalid identifiers should fail before any HTTP request")
	})
}
