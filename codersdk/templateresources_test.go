package codersdk_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/codersdk"
)

func TestCreateTemplateResourceRevision(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		req  codersdk.CreateTemplateResourceRevisionRequest
	}{
		{"source", codersdk.CreateTemplateResourceRevisionRequest{Source: "# main.tf"}},
		{"archive", codersdk.CreateTemplateResourceRevisionRequest{FileID: uuid.New()}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			orgID, foundationID, revisionID, fileID := uuid.New(), uuid.New(), uuid.New(), uuid.New()
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPost || r.URL.Path != "/api/v2/organizations/"+orgID.String()+"/foundations/"+foundationID.String()+"/versions" {
					t.Error("unexpected revision request", r.Method, r.URL.Path)
					w.WriteHeader(http.StatusBadRequest)
					return
				}
				var body map[string]json.RawMessage
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Error(err)
					w.WriteHeader(http.StatusBadRequest)
					return
				}
				if tc.req.FileID != uuid.Nil {
					if _, exists := body["source"]; exists {
						t.Error("archive requests should omit empty source")
					}
					var requestedID uuid.UUID
					if err := json.Unmarshal(body["file_id"], &requestedID); err != nil || requestedID != tc.req.FileID {
						t.Error("incorrect file_id", requestedID, err)
					}
				} else {
					var source string
					if err := json.Unmarshal(body["source"], &source); err != nil || source != tc.req.Source {
						t.Error("incorrect source", source, err)
					}
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusCreated)
				if err := json.NewEncoder(w).Encode(codersdk.TemplateResourceRevision{ID: revisionID, FoundationID: foundationID, FileID: fileID, Source: tc.req.Source}); err != nil {
					t.Error(err)
				}
			}))
			t.Cleanup(server.Close)
			serverURL, err := url.Parse(server.URL)
			require.NoError(t, err)
			client := codersdk.New(serverURL)
			revision, err := client.CreateFoundationRevision(t.Context(), orgID, foundationID, tc.req)
			require.NoError(t, err)
			require.Equal(t, revisionID, revision.ID)
			require.Equal(t, fileID, revision.FileID, "response identifies the normalized snapshot, not the upload")
			require.Equal(t, tc.req.Source, revision.Source)
		})
	}
}
