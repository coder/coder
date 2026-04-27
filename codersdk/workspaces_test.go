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

	// writeJSON is a small helper that writes a JSON-encoded value
	// with the given status code.
	writeJSON := func(w http.ResponseWriter, status int, v any) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(v)
	}

	// errResponse builds a codersdk.Response suitable for error
	// replies.
	errResponse := func(msg string) codersdk.Response {
		return codersdk.Response{Message: msg}
	}

	// newWorkspace returns a Workspace with the given ID and name.
	newWorkspace := func(id uuid.UUID, name string) codersdk.Workspace {
		return codersdk.Workspace{ID: id, Name: name}
	}

	// Each table case configures a mock server with separate UUID
	// and name endpoint behaviors, then calls ResolveWorkspace with
	// the given identifier.
	type endpointResponse struct {
		status    int
		workspace codersdk.Workspace
		errMsg    string
	}
	tests := []struct {
		name       string
		identifier string
		// uuidEndpoint configures GET /api/v2/workspaces/{workspace}.
		// nil means the endpoint is not registered (404 from chi).
		uuidEndpoint *endpointResponse
		// nameEndpoint configures GET /api/v2/users/{user}/workspace/{workspace}.
		// nil means the endpoint is not registered.
		nameEndpoint *endpointResponse
		// expectedOwner and expectedName are checked via assert inside
		// the name endpoint handler (when non-empty).
		expectedOwner string
		expectedName  string
		// Expected outcomes.
		wantErr        bool
		wantStatusCode int
		wantUUIDHits   int64
		wantNameHits   int64
	}{
		{
			name:       "ByUUID",
			identifier: "", // filled dynamically below
			uuidEndpoint: &endpointResponse{
				status: http.StatusOK,
			},
			wantUUIDHits: 1,
			wantNameHits: 0,
		},
		{
			name:       "ByName",
			identifier: "my-workspace",
			nameEndpoint: &endpointResponse{
				status: http.StatusOK,
			},
			expectedOwner: "me",
			expectedName:  "my-workspace",
			wantUUIDHits:  0,
			wantNameHits:  1,
		},
		{
			name:       "ByOwnerAndName",
			identifier: "alice/my-workspace",
			nameEndpoint: &endpointResponse{
				status: http.StatusOK,
			},
			expectedOwner: "alice",
			expectedName:  "my-workspace",
			wantUUIDHits:  0,
			wantNameHits:  1,
		},
		{
			name:       "OwnerWithUUIDLikeName",
			identifier: "alice/a1b2c3d4e5f6a7b8c9d0e1f2a3b4c5d6",
			nameEndpoint: &endpointResponse{
				status: http.StatusOK,
			},
			expectedOwner: "alice",
			expectedName:  "a1b2c3d4e5f6a7b8c9d0e1f2a3b4c5d6",
			wantUUIDHits:  0,
			wantNameHits:  1,
		},
		{
			name:       "UUIDLikeNameFallback",
			identifier: "a1b2c3d4e5f6a7b8c9d0e1f2a3b4c5d6",
			uuidEndpoint: &endpointResponse{
				status: http.StatusNotFound,
				errMsg: "Resource not found.",
			},
			nameEndpoint: &endpointResponse{
				status: http.StatusOK,
			},
			expectedOwner: "me",
			expectedName:  "a1b2c3d4e5f6a7b8c9d0e1f2a3b4c5d6",
			wantUUIDHits:  1,
			wantNameHits:  1,
		},
		{
			name:       "DashedUUIDNotFound",
			identifier: "", // filled dynamically (standard dashed UUID)
			uuidEndpoint: &endpointResponse{
				status: http.StatusNotFound,
				errMsg: "Resource not found.",
			},
			nameEndpoint: &endpointResponse{
				status: http.StatusNotFound,
				errMsg: "Resource not found.",
			},
			wantErr:        true,
			wantStatusCode: http.StatusNotFound,
			// NameValid rejects dashed UUIDs (36 chars), so the
			// name endpoint should not be called.
			wantUUIDHits: 1,
			wantNameHits: 0,
		},
		{
			name:       "NonNotFoundError",
			identifier: "", // filled dynamically
			uuidEndpoint: &endpointResponse{
				status: http.StatusInternalServerError,
				errMsg: "Internal server error.",
			},
			nameEndpoint: &endpointResponse{
				status: http.StatusOK,
			},
			wantErr:        true,
			wantStatusCode: http.StatusInternalServerError,
			wantUUIDHits:   1,
			wantNameHits:   0,
		},
		{
			name:       "NameNotFound",
			identifier: "nonexistent",
			nameEndpoint: &endpointResponse{
				status: http.StatusNotFound,
				errMsg: "Resource not found.",
			},
			expectedOwner:  "me",
			expectedName:   "nonexistent",
			wantErr:        true,
			wantStatusCode: http.StatusNotFound,
			wantUUIDHits:   0,
			wantNameHits:   1,
		},
		{
			name:       "Forbidden",
			identifier: "", // filled dynamically
			uuidEndpoint: &endpointResponse{
				status: http.StatusForbidden,
				errMsg: "Forbidden.",
			},
			nameEndpoint: &endpointResponse{
				status: http.StatusOK,
			},
			wantErr:        true,
			wantStatusCode: http.StatusForbidden,
			wantUUIDHits:   1,
			wantNameHits:   0,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			wsID := uuid.New()
			expected := newWorkspace(wsID, "test-workspace")

			// When identifier is empty, use the workspace UUID
			// (standard dashed format).
			identifier := tt.identifier
			if identifier == "" {
				identifier = wsID.String()
			}

			var uuidHits, nameHits atomic.Int64
			r := chi.NewRouter()

			if tt.uuidEndpoint != nil {
				ep := tt.uuidEndpoint
				// Use the expected workspace in OK responses
				// unless the test overrides it.
				if ep.status == http.StatusOK && ep.workspace.ID == uuid.Nil {
					ep.workspace = expected
				}
				r.Get("/api/v2/workspaces/{workspace}", func(w http.ResponseWriter, req *http.Request) {
					uuidHits.Add(1)
					if ep.errMsg != "" {
						writeJSON(w, ep.status, errResponse(ep.errMsg))
						return
					}
					writeJSON(w, ep.status, ep.workspace)
				})
			}

			if tt.nameEndpoint != nil {
				ep := tt.nameEndpoint
				if ep.status == http.StatusOK && ep.workspace.ID == uuid.Nil {
					ep.workspace = expected
				}
				r.Get("/api/v2/users/{user}/workspace/{workspace}", func(w http.ResponseWriter, req *http.Request) {
					nameHits.Add(1)
					if tt.expectedOwner != "" {
						assert.Equal(t, tt.expectedOwner, chi.URLParam(req, "user"))
					}
					if tt.expectedName != "" {
						assert.Equal(t, tt.expectedName, chi.URLParam(req, "workspace"))
					}
					if ep.errMsg != "" {
						writeJSON(w, ep.status, errResponse(ep.errMsg))
						return
					}
					writeJSON(w, ep.status, ep.workspace)
				})
			}

			srv := httptest.NewServer(r)
			defer srv.Close()

			u, err := url.Parse(srv.URL)
			require.NoError(t, err)
			client := codersdk.New(u)

			ws, err := client.ResolveWorkspace(t.Context(), identifier)
			if tt.wantErr {
				require.Error(t, err)
				if tt.wantStatusCode != 0 {
					var sdkErr *codersdk.Error
					require.ErrorAs(t, err, &sdkErr)
					require.Equal(t, tt.wantStatusCode, sdkErr.StatusCode())
				}
			} else {
				require.NoError(t, err)
				require.Equal(t, expected.ID, ws.ID)
			}

			require.EqualValues(t, tt.wantUUIDHits, uuidHits.Load())
			require.EqualValues(t, tt.wantNameHits, nameHits.Load())
		})
	}

	// Cases that need a structurally different server setup.

	t.Run("TransportError", func(t *testing.T) {
		t.Parallel()

		// Close the server immediately so the transport layer fails.
		srv := httptest.NewServer(http.NotFoundHandler())
		srvURL, err := url.Parse(srv.URL)
		require.NoError(t, err)
		srv.Close()

		client := codersdk.New(srvURL)

		_, err = client.ResolveWorkspace(t.Context(), uuid.NewString())
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
