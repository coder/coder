package aibridged

import (
	"bufio"
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"sync"
	"time"

	"golang.org/x/xerrors"
)

// ResponseInterceptFunc is called for each chunk of data received from the upstream server.
// For streaming responses, it's called for each SSE event or data chunk.
// For non-streaming responses, it's called once with the complete response body.
// The function receives a copy of the data - modifications don't affect the response.
type ResponseInterceptFunc func(sess *OpenAISession, data []byte, isStreaming bool) ([][]byte, bool, error)

// RequestInterceptFunc is called for each request before it's sent to the upstream server.
// The function receives a copy of the request body - modifications don't affect the request.
type RequestInterceptFunc func(req *http.Request, body []byte) error

// RequestModifyFunc is called to modify the request body before sending to upstream.
// Unlike RequestInterceptFunc, this function can modify the request body.
// Returns the modified body or an error.
type RequestModifyFunc func(req *http.Request, body []byte) ([]byte, error)

// SSEProxy provides a fast, memory-efficient proxy using httputil.ReverseProxy.
// It supports efficient copying of responses for interception without buffering.
type SSEProxy struct {
	*httputil.ReverseProxy
	responseInterceptFunc ResponseInterceptFunc
	requestInterceptFunc  RequestInterceptFunc
	requestModifyFunc     RequestModifyFunc
	bufferPool            sync.Pool
	config                *ProxyConfig
}

// NewSSEProxy creates a new SSE proxy that proxies to the given target URL.
func NewSSEProxy(target *url.URL, responseInterceptFunc ResponseInterceptFunc) *SSEProxy {
	return NewSSEProxyWithRequestIntercept(target, responseInterceptFunc, nil)
}

// NewSSEProxyWithRequestIntercept creates a new SSE proxy with both request and response interception.
func NewSSEProxyWithRequestIntercept(target *url.URL, responseInterceptFunc ResponseInterceptFunc, requestInterceptFunc RequestInterceptFunc) *SSEProxy {
	proxy := &SSEProxy{
		responseInterceptFunc: responseInterceptFunc,
		requestInterceptFunc:  requestInterceptFunc,
		bufferPool: sync.Pool{
			New: func() interface{} {
				// Use 32KB buffers for optimal performance
				return make([]byte, 32*1024)
			},
		},
	}

	proxy.ReverseProxy = httputil.NewSingleHostReverseProxy(target)

	// Configure for optimal streaming performance
	proxy.FlushInterval = -1 // Immediate flushing

	// Custom transport for streaming optimization
	proxy.Transport = &http.Transport{
		DisableCompression:    true, // Disable compression for streaming
		MaxIdleConns:          100,
		MaxIdleConnsPerHost:   10,
		ResponseHeaderTimeout: 0, // No timeout for streaming responses
	}

	// Custom director for request interception
	originalDirector := proxy.Director
	proxy.Director = func(req *http.Request) {
		originalDirector(req)

		// Intercept request if function is provided
		if proxy.requestInterceptFunc != nil {
			proxy.interceptRequest(req)
		}
	}

	// Custom response modifier for interception
	originalModifyResponse := proxy.ModifyResponse
	proxy.ModifyResponse = func(resp *http.Response) error {
		// Call original modifier first if it exists
		if originalModifyResponse != nil {
			if err := originalModifyResponse(resp); err != nil {
				return err
			}
		}

		// Wrap the response body for interception
		resp.Body = proxy.wrapResponseBody(resp.Body, resp.Header)
		return nil
	}

	return proxy
}

// interceptRequest intercepts and processes the request body
func (p *SSEProxy) interceptRequest(req *http.Request) {
	if req.Body == nil {
		// Call intercept function with empty body
		if p.requestInterceptFunc != nil {
			if err := p.requestInterceptFunc(req, nil); err != nil {
				// Log error but continue
				_ = err
			}
		}
		return
	}

	// Read the request body
	body, err := io.ReadAll(req.Body)
	if err != nil {
		// Log error but continue
		_ = err
		return
	}

	// Close the original body
	req.Body.Close()

	// Create a copy for interception
	bodyCopy := make([]byte, len(body))
	copy(bodyCopy, body)

	// Call intercept function if provided
	if p.requestInterceptFunc != nil {
		if err := p.requestInterceptFunc(req, bodyCopy); err != nil {
			// Log error but continue
			_ = err
		}
	}

	// Apply request modifications if provided
	finalBody := body
	if p.requestModifyFunc != nil {
		modifiedBody, err := p.requestModifyFunc(req, bodyCopy)
		if err != nil {
			// Log error but continue with original body
			_ = err
		} else {
			finalBody = modifiedBody
		}
	}

	// Restore the request body for the upstream request
	req.Body = io.NopCloser(bytes.NewReader(finalBody))
	req.ContentLength = int64(len(finalBody))
}

// wrapResponseBody wraps the response body to enable interception
func (p *SSEProxy) wrapResponseBody(body io.ReadCloser, headers http.Header) io.ReadCloser {
	if p.responseInterceptFunc == nil {
		return body
	}

	// Determine if this is a streaming response
	contentType := headers.Get("Content-Type")
	isStreaming := strings.Contains(contentType, "text/event-stream") ||
		strings.Contains(contentType, "text/plain") ||
		headers.Get("Transfer-Encoding") == "chunked"

	if isStreaming {
		return &streamingInterceptReader{
			ReadCloser:    body,
			interceptFunc: p.responseInterceptFunc,
			bufferPool:    &p.bufferPool,
			sess:          p.config.OpenAISession,
		}
	}

	return &nonStreamingInterceptReader{
		ReadCloser:    body,
		interceptFunc: p.responseInterceptFunc,
	}
}

// streamingInterceptReader intercepts streaming responses chunk by chunk
type streamingInterceptReader struct {
	io.ReadCloser
	interceptFunc ResponseInterceptFunc
	bufferPool    *sync.Pool
	reader        *bufio.Reader
	readerOnce    sync.Once

	sess          *OpenAISession
	trailerEvents [][]byte
}

func (sir *streamingInterceptReader) Read(p []byte) (n int, err error) {
	// Initialize buffered reader once
	sir.readerOnce.Do(func() {
		sir.reader = bufio.NewReader(sir.ReadCloser)
	})

	for {
		n, err = sir.reader.Read(p)
		if n > 0 {
			// Call intercept function with a copy of the chunk
			chunk := make([]byte, n)
			copy(chunk, p[:n])

			// Once [DONE] is found, we need to inject the trailer events and rewind the reader.
			if bytes.Contains(chunk, []byte(`[DONE]`)) {
				// TODO: inject trailers then [DONE] events.
				//continue
			}

			extraEvents, send, interceptErr := sir.interceptFunc(sir.sess, chunk, true)
			if interceptErr != nil {
				// TODO: Log error but continue reading
				_ = interceptErr
			}

			if !send {
				continue
			}
			sir.trailerEvents = extraEvents
		}

		break
	}
	return n, err
}

// nonStreamingInterceptReader intercepts complete non-streaming responses
type nonStreamingInterceptReader struct {
	io.ReadCloser
	interceptFunc ResponseInterceptFunc
	buffer        bytes.Buffer
	intercepted   bool
	mu            sync.Mutex
}

func (nsir *nonStreamingInterceptReader) Read(p []byte) (n int, err error) {
	n, err = nsir.ReadCloser.Read(p)

	if n > 0 {
		nsir.mu.Lock()
		nsir.buffer.Write(p[:n])
		nsir.mu.Unlock()
	}

	// If we've reached EOF and haven't intercepted yet, do it now
	if err == io.EOF && !nsir.intercepted {
		nsir.mu.Lock()
		if !nsir.intercepted {
			nsir.intercepted = true
			data := nsir.buffer.Bytes()
			nsir.mu.Unlock()

			if _, _, interceptErr := nsir.interceptFunc(nil, data, false); interceptErr != nil {
				// Log error but continue
				_ = interceptErr
			}
		} else {
			nsir.mu.Unlock()
		}
	}

	return n, err
}

// ProxyConfig holds configuration for the SSE proxy
type ProxyConfig struct {
	Target                *url.URL
	ResponseInterceptFunc ResponseInterceptFunc
	RequestInterceptFunc  RequestInterceptFunc
	RequestModifyFunc     RequestModifyFunc
	ModifyRequest         func(*http.Request)
	ModifyResponse        func(*http.Response) error
	ErrorHandler          func(http.ResponseWriter, *http.Request, error)
	Transport             http.RoundTripper
	FlushInterval         time.Duration // -1 for immediate flushing

	OpenAISession *OpenAISession
}

// NewSSEProxyWithConfig creates a new SSE proxy with custom configuration
func NewSSEProxyWithConfig(config ProxyConfig) (*SSEProxy, error) {
	if config.Target == nil {
		return nil, xerrors.Errorf("target URL is required")
	}

	proxy := &SSEProxy{
		responseInterceptFunc: config.ResponseInterceptFunc,
		requestInterceptFunc:  config.RequestInterceptFunc,
		requestModifyFunc:     config.RequestModifyFunc,
		bufferPool: sync.Pool{
			New: func() interface{} {
				return make([]byte, 32*1024)
			},
		},

		config: &config,
	}

	proxy.ReverseProxy = httputil.NewSingleHostReverseProxy(config.Target)

	// Configure flush interval
	if config.FlushInterval != 0 {
		proxy.FlushInterval = config.FlushInterval
	} else {
		proxy.FlushInterval = -1 // Default to immediate flushing
	}

	// Configure transport
	if config.Transport != nil {
		proxy.Transport = config.Transport
	} else {
		proxy.Transport = &http.Transport{
			DisableCompression:    true,
			MaxIdleConns:          100,
			MaxIdleConnsPerHost:   10,
			ResponseHeaderTimeout: 0,
		}
	}

	// Configure request modifier and interception
	originalDirector := proxy.Director
	proxy.Director = func(req *http.Request) {
		originalDirector(req)

		req.Host = config.Target.Host
		req.URL.Scheme = config.Target.Scheme
		req.URL.Host = config.Target.Host

		// Apply custom request modifications first
		if config.ModifyRequest != nil {
			config.ModifyRequest(req)
		}

		// Then apply request interception and modification
		if proxy.requestInterceptFunc != nil || proxy.requestModifyFunc != nil {
			proxy.interceptRequest(req)
		}
	}

	// Configure response modifier
	proxy.ModifyResponse = func(resp *http.Response) error {
		// Call custom modifier first if it exists
		if config.ModifyResponse != nil {
			if err := config.ModifyResponse(resp); err != nil {
				return err
			}
		}

		// Wrap the response body for interception
		resp.Body = proxy.wrapResponseBody(resp.Body, resp.Header)
		return nil
	}

	// Configure error handler
	if config.ErrorHandler != nil {
		proxy.ErrorHandler = config.ErrorHandler
	}

	return proxy, nil
}

// ServeHTTPWithContext serves HTTP requests with context for cancellation
func (p *SSEProxy) ServeHTTPWithContext(ctx context.Context, w http.ResponseWriter, r *http.Request) {
	// Create a new request with the given context
	r = r.WithContext(ctx)

	// Set up cancellation monitoring
	done := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			// Context cancelled, try to close the connection
			if hijacker, ok := w.(http.Hijacker); ok {
				if conn, _, err := hijacker.Hijack(); err == nil {
					conn.Close()
				}
			}
		case <-done:
			// Request completed normally
		}
	}()

	// Serve the request
	p.ServeHTTP(w, r)
	close(done)
}
