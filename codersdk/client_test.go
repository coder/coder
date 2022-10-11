package codersdk_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/coder/coder/codersdk"
)

const (
	jsonCT = "application/json"
)

func Test_readBodyAsError(t *testing.T) {
	simpleResponse := codersdk.Response{
		Message: "test",
		Detail:  "hi",
	}

	tests := []struct {
		name        string
		req         *http.Request
		res         *http.Response
		errContains error
		assert      func(t *testing.T, err *codersdk.Error)
	}{
		{
			name: "StandardWithRequest",
			req:  httptest.NewRequest(http.MethodGet, "http://example.com", nil),
			res:  newResponse(http.StatusNotFound, jsonCT, responseString(simpleResponse)),
			assert: func(t *testing.T, err *codersdk.Error) {
				assert.Equal(t, http.StatusNotFound, err.StatusCode())
				assert.Equal(t, simpleResponse, err.Response)
				// assert.Equal(t, http.MethodGet, err.)
			},
		},
	}
	for _, c := range tests {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			// TODO: this
		})
	}
}

func newResponse(status int, contentType string, body interface{}) *http.Response {
	var r io.ReadCloser
	switch v := body.(type) {
	case string:
		r = io.NopCloser(strings.NewReader(v))
	case []byte:
		r = io.NopCloser(bytes.NewReader(v))
	case io.ReadCloser:
		r = v
	case io.Reader:
		r = io.NopCloser(v)
	default:
		panic(fmt.Sprintf("unknown body type: %T", body))
	}

	return &http.Response{
		Status:     http.StatusText(status),
		StatusCode: status,
		Header: http.Header{
			"Content-Type": []string{contentType},
		},
		Body: r,
	}
}

func responseString(res codersdk.Response) string {
	b, err := json.Marshal(res)
	if err != nil {
		panic(err)
	}

	return string(b)
}
