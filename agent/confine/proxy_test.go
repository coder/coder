package confine_test

import (
	"bufio"
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/agent/confine"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/agentsdk"
	"github.com/coder/coder/v2/testutil"
)

func TestProxyCONNECTAllowedTLS(t *testing.T) {
	t.Parallel()

	upstream := httptest.NewTLSServer(http.HandlerFunc(func(rw http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(rw, "tunneled")
	}))
	defer upstream.Close()
	upstreamURL, err := url.Parse(upstream.URL)
	require.NoError(t, err)
	host, port := splitHostPort(t, upstreamURL.Host)

	engine := confine.NewPolicyEngine("", 0)
	engine.Update(codersdk.AIEgressPolicy{Revision: 3, Rules: []codersdk.AIEgressRule{{Host: host, Ports: []int{port}}}})
	events := make(chan confine.NetworkEvent, 1)
	proxy, err := confine.ListenProxy("127.0.0.1:0", engine, func(event confine.NetworkEvent) { events <- event })
	require.NoError(t, err)
	defer proxy.Close()

	conn, err := net.Dial("tcp", proxy.Addr().String())
	require.NoError(t, err)
	defer conn.Close()
	_, err = fmt.Fprintf(conn, "CONNECT %s HTTP/1.1\r\nHost: %s\r\n\r\n", upstreamURL.Host, upstreamURL.Host)
	require.NoError(t, err)
	res, err := http.ReadResponse(bufio.NewReader(conn), &http.Request{Method: http.MethodConnect})
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, res.StatusCode)
	require.NoError(t, res.Body.Close())

	//nolint:gosec // The test upstream uses an ephemeral self-signed certificate.
	tlsConn := tls.Client(conn, &tls.Config{InsecureSkipVerify: true})
	require.NoError(t, tlsConn.Handshake())
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, upstream.URL, nil)
	require.NoError(t, err)
	require.NoError(t, req.Write(tlsConn))
	response, err := http.ReadResponse(bufio.NewReader(tlsConn), req)
	require.NoError(t, err)
	body, err := io.ReadAll(response.Body)
	require.NoError(t, err)
	require.NoError(t, response.Body.Close())
	require.Equal(t, "tunneled", string(body))

	event := <-events
	require.Equal(t, agentsdk.AISandboxNetworkProtocolConnect, event.Protocol)
	require.Equal(t, agentsdk.AISandboxNetworkEventActionAllowed, event.Action)
	require.Equal(t, host, event.Host)
	require.Equal(t, port, event.Port)
	require.EqualValues(t, 3, event.PolicyRevision)
}

func TestProxyCONNECTDenied(t *testing.T) {
	t.Parallel()

	engine := confine.NewPolicyEngine("", 0)
	events := make(chan confine.NetworkEvent, 1)
	proxy, err := confine.ListenProxy("127.0.0.1:0", engine, func(event confine.NetworkEvent) { events <- event })
	require.NoError(t, err)
	defer proxy.Close()

	conn, err := net.Dial("tcp", proxy.Addr().String())
	require.NoError(t, err)
	defer conn.Close()
	_, err = fmt.Fprint(conn, "CONNECT denied.example.com:443 HTTP/1.1\r\nHost: denied.example.com:443\r\n\r\n")
	require.NoError(t, err)
	res, err := http.ReadResponse(bufio.NewReader(conn), &http.Request{Method: http.MethodConnect})
	require.NoError(t, err)
	defer res.Body.Close()
	body, err := io.ReadAll(res.Body)
	require.NoError(t, err)
	require.Equal(t, http.StatusForbidden, res.StatusCode)
	require.Contains(t, string(body), "denied.example.com")

	event := <-events
	require.Equal(t, agentsdk.AISandboxNetworkEventActionDenied, event.Action)
	require.EqualValues(t, 0, event.PolicyRevision)
}

func TestProxyHTTPForward(t *testing.T) {
	t.Parallel()

	upstream := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		require.Equal(t, "/hello", req.URL.Path)
		rw.Header().Set("X-Upstream", "yes")
		_, _ = io.WriteString(rw, "forwarded")
	}))
	defer upstream.Close()
	upstreamURL, err := url.Parse(upstream.URL)
	require.NoError(t, err)
	host, port := splitHostPort(t, upstreamURL.Host)

	engine := confine.NewPolicyEngine("", 0)
	engine.Update(codersdk.AIEgressPolicy{Revision: 9, Rules: []codersdk.AIEgressRule{{Host: host, Ports: []int{port}}}})
	events := make(chan confine.NetworkEvent, 1)
	proxy, err := confine.ListenProxy("127.0.0.1:0", engine, func(event confine.NetworkEvent) { events <- event })
	require.NoError(t, err)
	defer proxy.Close()
	proxyURL, err := url.Parse("http://" + proxy.Addr().String())
	require.NoError(t, err)

	client := &http.Client{Transport: &http.Transport{Proxy: http.ProxyURL(proxyURL)}}
	req, err := http.NewRequestWithContext(testutil.Context(t, testutil.WaitShort), http.MethodGet, upstream.URL+"/hello", nil)
	require.NoError(t, err)
	res, err := client.Do(req)
	require.NoError(t, err)
	defer res.Body.Close()
	body, err := io.ReadAll(res.Body)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, res.StatusCode)
	require.Equal(t, "yes", res.Header.Get("X-Upstream"))
	require.Equal(t, "forwarded", string(body))

	event := <-events
	require.Equal(t, agentsdk.AISandboxNetworkProtocolHTTP, event.Protocol)
	require.Equal(t, agentsdk.AISandboxNetworkEventActionAllowed, event.Action)
	require.EqualValues(t, 9, event.PolicyRevision)
}

func TestProxyPolicyAtomicUpdate(t *testing.T) {
	t.Parallel()

	upstream := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, _ *http.Request) {
		rw.WriteHeader(http.StatusNoContent)
	}))
	defer upstream.Close()
	upstreamURL, err := url.Parse(upstream.URL)
	require.NoError(t, err)
	host, port := splitHostPort(t, upstreamURL.Host)

	engine := confine.NewPolicyEngine("", 0)
	allow := codersdk.AIEgressPolicy{Revision: 1, Rules: []codersdk.AIEgressRule{{Host: host, Ports: []int{port}}}}
	deny := codersdk.AIEgressPolicy{Revision: 2}
	engine.Update(allow)
	proxy, err := confine.ListenProxy("127.0.0.1:0", engine, nil)
	require.NoError(t, err)
	defer proxy.Close()
	proxyURL, err := url.Parse("http://" + proxy.Addr().String())
	require.NoError(t, err)

	ctx := testutil.Context(t, testutil.WaitMedium)
	stop := make(chan struct{})
	started := make(chan struct{}, 8)
	var wg sync.WaitGroup
	errs := make(chan error, 8)
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			client := &http.Client{Transport: &http.Transport{Proxy: http.ProxyURL(proxyURL), DisableKeepAlives: true}}
			started <- struct{}{}
			for {
				select {
				case <-stop:
					return
				default:
				}
				req, err := http.NewRequestWithContext(ctx, http.MethodGet, upstream.URL, nil)
				if err != nil {
					errs <- err
					return
				}
				res, err := client.Do(req)
				if err != nil {
					errs <- err
					return
				}
				_ = res.Body.Close()
				if res.StatusCode != http.StatusNoContent && res.StatusCode != http.StatusForbidden {
					errs <- xerrors.Errorf("unexpected status %d", res.StatusCode)
					return
				}
			}
		}()
	}
	for range 8 {
		<-started
	}
	for range 100 {
		engine.Update(deny)
		engine.Update(allow)
	}
	close(stop)
	wg.Wait()
	close(errs)
	for err := range errs {
		require.NoError(t, err)
	}
}

func splitHostPort(t *testing.T, authority string) (string, int) {
	t.Helper()
	host, portString, err := net.SplitHostPort(authority)
	require.NoError(t, err)
	port, err := strconv.Atoi(portString)
	require.NoError(t, err)
	return host, port
}
