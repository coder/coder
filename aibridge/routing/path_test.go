package routing_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/coder/coder/v2/aibridge/routing"
)

func TestRejectInvalidForwardPath(t *testing.T) {
	t.Parallel()

	var called bool
	handler := routing.RejectInvalidForwardPath(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/openai/v1/models/%2e%2e/files", nil))
	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Contains(t, rec.Body.String(), routing.InvalidPathMessage)
	assert.False(t, called)

	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/openai/v1/models/org%2Fmodel", nil))
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.True(t, called)
}
