package httpapi_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/coderd/httpapi"
)

func TestWrite(t *testing.T) {
	t.Parallel()
	t.Run("NoErrors", func(t *testing.T) {
		t.Parallel()
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

func TestRead(t *testing.T) {
	t.Parallel()
	t.Run("EmptyStruct", func(t *testing.T) {
		t.Parallel()
		rw := httptest.NewRecorder()
		r := httptest.NewRequest("POST", "/", bytes.NewBufferString("{}"))
		v := struct{}{}
		require.True(t, httpapi.Read(rw, r, &v))
	})

	t.Run("NoBody", func(t *testing.T) {
		t.Parallel()
		rw := httptest.NewRecorder()
		r := httptest.NewRequest("POST", "/", nil)
		var v json.RawMessage
		require.False(t, httpapi.Read(rw, r, v))
	})

	t.Run("Validate", func(t *testing.T) {
		t.Parallel()
		type toValidate struct {
			Value string `json:"value" validate:"required"`
		}
		rw := httptest.NewRecorder()
		r := httptest.NewRequest("POST", "/", bytes.NewBufferString(`{"value":"hi"}`))

		var validate toValidate
		require.True(t, httpapi.Read(rw, r, &validate))
		require.Equal(t, validate.Value, "hi")
	})

	t.Run("ValidateFailure", func(t *testing.T) {
		t.Parallel()
		type toValidate struct {
			Value string `json:"value" validate:"required"`
		}
		rw := httptest.NewRecorder()
		r := httptest.NewRequest("POST", "/", bytes.NewBufferString("{}"))

		var validate toValidate
		require.False(t, httpapi.Read(rw, r, &validate))
		var v httpapi.Response
		err := json.NewDecoder(rw.Body).Decode(&v)
		require.NoError(t, err)
		require.Len(t, v.Errors, 1)
		require.Equal(t, v.Errors[0].Field, "value")
		require.Equal(t, v.Errors[0].Code, "required")
	})
}

func TestReadUsername(t *testing.T) {
	t.Parallel()
	// Tests whether usernames are valid or not.
	testCases := []struct {
		Username string
		Valid    bool
	}{
		{"1", true},
		{"12", true},
		{"123", true},
		{"12345678901234567890", true},
		{"123456789012345678901", true},
		{"a", true},
		{"a1", true},
		{"a1b2", true},
		{"a1b2c3d4e5f6g7h8i9j0", true},
		{"a1b2c3d4e5f6g7h8i9j0k", true},
		{"aa", true},
		{"abc", true},
		{"abcdefghijklmnopqrst", true},
		{"abcdefghijklmnopqrstu", true},
		{"wow-test", true},

		{"", false},
		{" ", false},
		{" a", false},
		{" a ", false},
		{" 1", false},
		{"1 ", false},
		{" aa", false},
		{"aa ", false},
		{" 12", false},
		{"12 ", false},
		{" a1", false},
		{"a1 ", false},
		{" abcdefghijklmnopqrstu", false},
		{"abcdefghijklmnopqrstu ", false},
		{" 123456789012345678901", false},
		{" a1b2c3d4e5f6g7h8i9j0k", false},
		{"a1b2c3d4e5f6g7h8i9j0k ", false},
		{"bananas_wow", false},
		{"test--now", false},

		{"123456789012345678901234567890123", false},
		{"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", false},
		{"123456789012345678901234567890123123456789012345678901234567890123", false},
	}
	type toValidate struct {
		Username string `json:"username" validate:"username"`
	}
	for _, testCase := range testCases {
		testCase := testCase
		t.Run(testCase.Username, func(t *testing.T) {
			t.Parallel()
			rw := httptest.NewRecorder()
			data, err := json.Marshal(toValidate{testCase.Username})
			require.NoError(t, err)
			r := httptest.NewRequest("POST", "/", bytes.NewBuffer(data))

			var validate toValidate
			require.Equal(t, httpapi.Read(rw, r, &validate), testCase.Valid)
		})
	}
}
