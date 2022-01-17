package httpapi_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/httpapi"
)

func TestResponse(t *testing.T) {
	t.Run("NoErrors", func(t *testing.T) {
		rw := httptest.NewRecorder()
		httpapi.Write(rw, http.StatusOK, httpapi.Response{
			Message: "wow",
		})
		var m map[string]interface{}
		err := json.NewDecoder(rw.Body).Decode(&m)
		require.NoError(t, err)
		_, ok := m["errors"]
		require.False(t, ok)
	})
}
