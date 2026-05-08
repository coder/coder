package httpmw_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/serpent"
)

func TestRequireTemplateBuilderEnabled(t *testing.T) {
	t.Parallel()

	t.Run("Disabled", func(t *testing.T) {
		t.Parallel()

		var enabled serpent.Bool
		handler := httpmw.RequireTemplateBuilderEnabled(&enabled)(
			http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusOK)
			}),
		)

		r := httptest.NewRequest(http.MethodGet, "/", nil)
		rw := httptest.NewRecorder()
		handler.ServeHTTP(rw, r)

		require.Equal(t, http.StatusNotFound, rw.Code)

		var resp codersdk.Response
		err := json.NewDecoder(rw.Body).Decode(&resp)
		require.NoError(t, err)
		require.Equal(t, "Template builder is not enabled.", resp.Message)
	})

	t.Run("Enabled", func(t *testing.T) {
		t.Parallel()

		var enabled serpent.Bool
		err := enabled.Set("true")
		require.NoError(t, err)

		handler := httpmw.RequireTemplateBuilderEnabled(&enabled)(
			http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusOK)
			}),
		)

		r := httptest.NewRequest(http.MethodGet, "/", nil)
		rw := httptest.NewRecorder()
		handler.ServeHTTP(rw, r)

		require.Equal(t, http.StatusOK, rw.Code)
	})
}
