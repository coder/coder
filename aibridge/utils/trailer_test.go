package utils_test

import (
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/aibridge/utils"
)

func TestDropRequestTrailers(t *testing.T) {
	t.Parallel()

	req := &http.Request{Trailer: http.Header{"X-Secret": {"value"}}}
	utils.DropRequestTrailers(req)
	require.Nil(t, req.Trailer)
}

func TestDropResponseTrailers(t *testing.T) {
	t.Parallel()

	resp := &http.Response{Trailer: http.Header{"Set-Cookie": {"early"}}}
	resp.Body = &trailerPopulatingBody{
		Reader:   strings.NewReader("body"),
		response: resp,
	}

	utils.DropResponseTrailers(resp)
	require.Nil(t, resp.Trailer)

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Equal(t, "body", string(body))
	require.Nil(t, resp.Trailer)

	require.NoError(t, resp.Body.Close())
	require.Nil(t, resp.Trailer)
}

type trailerPopulatingBody struct {
	io.Reader
	response *http.Response
}

func (b *trailerPopulatingBody) Read(p []byte) (int, error) {
	n, err := b.Reader.Read(p)
	if err == io.EOF {
		b.response.Trailer = http.Header{"Set-Cookie": {"late"}}
	}
	return n, err
}

func (b *trailerPopulatingBody) Close() error {
	b.response.Trailer = http.Header{"Set-Cookie": {"close"}}
	return nil
}
