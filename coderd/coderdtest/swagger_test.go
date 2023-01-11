package coderdtest

import (
	"net/http"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/require"
)

func TestAllEndpointsDocumented(t *testing.T) {
	_, _, api := NewWithAPI(t, nil)

	err := chi.Walk(api.APIHandler, func(method, route string, handler http.Handler, middlewares ...func(http.Handler) http.Handler) error {
		return nil
	})
	require.NoError(t, err)
}
