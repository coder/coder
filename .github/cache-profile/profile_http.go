package main

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptrace"
	"os"
	"path/filepath"
	"runtime/pprof"
	"strings"
	"sync"
	"time"
)

// Diagnostics intentionally omit URLs, headers, object IDs, and credentials.
var profileLog struct {
	sync.Mutex
	w   *bufio.Writer
	f   *os.File
	cpu *os.File
}

func profileStart() func() {
	dir := os.Getenv("CACHE_PROFILE_DIR")
	if dir == "" {
		return func() {}
	}
	profileLog.f, _ = os.CreateTemp(dir, "cache-requests-*.jsonl")
	if profileLog.f == nil {
		return func() {}
	}
	profileLog.w = bufio.NewWriterSize(profileLog.f, 256<<10)
	profileLog.cpu, _ = os.CreateTemp(dir, "plugin-cpu-*.pprof")
	if profileLog.cpu != nil {
		_ = pprof.StartCPUProfile(profileLog.cpu)
	}
	return func() {
		if profileLog.cpu != nil {
			pprof.StopCPUProfile()
			_ = profileLog.cpu.Close()
		}
		profileLog.Lock()
		defer profileLog.Unlock()
		_ = profileLog.w.Flush()
		_ = profileLog.f.Close()
	}
}

func profileWrite(v any) {
	profileLog.Lock()
	defer profileLog.Unlock()
	if profileLog.w != nil {
		_ = json.NewEncoder(profileLog.w).Encode(v)
	}
}

type profileClient struct {
	base interface {
		Do(*http.Request) (*http.Response, error)
	}
}

func (c profileClient) Do(req *http.Request) (*http.Response, error) {
	start := time.Now()
	kind := "other"
	if strings.Contains(req.URL.Path, "/action/") {
		kind = "action"
	}
	if strings.Contains(req.URL.Path, "/output/") {
		kind = "output"
	}
	var mu sync.Mutex
	var firstByte float64
	var reused bool
	trace := &httptrace.ClientTrace{
		GotConn:              func(i httptrace.GotConnInfo) { mu.Lock(); reused = i.Reused; mu.Unlock() },
		GotFirstResponseByte: func() { mu.Lock(); firstByte = time.Since(start).Seconds(); mu.Unlock() },
	}
	resp, err := c.base.Do(req.WithContext(httptrace.WithClientTrace(req.Context(), trace)))
	if err != nil {
		profileWrite(map[string]any{"type": "http", "kind": kind, "method": req.Method, "start": start.UnixNano(), "seconds": time.Since(start).Seconds(), "error": true})
		return resp, err
	}
	resp.Body = &profileBody{ReadCloser: resp.Body, finish: func(n int64) {
		mu.Lock()
		fb, r := firstByte, reused
		mu.Unlock()
		profileWrite(map[string]any{"type": "http", "kind": kind, "method": req.Method, "status": resp.StatusCode, "start": start.UnixNano(), "seconds": time.Since(start).Seconds(), "ttfb": fb, "reused": r, "bytes": n})
	}}
	return resp, nil
}

type profileBody struct {
	io.ReadCloser
	n      int64
	finish func(int64)
	once   sync.Once
}

func (b *profileBody) Read(p []byte) (int, error) {
	n, e := b.ReadCloser.Read(p)
	b.n += int64(n)
	return n, e
}
func (b *profileBody) Close() error {
	e := b.ReadCloser.Close()
	b.once.Do(func() { b.finish(b.n) })
	return e
}

func profileGet(fn func(context.Context, string) (string, string, error)) func(context.Context, string) (string, string, error) {
	return func(ctx context.Context, key string) (string, string, error) {
		t := time.Now()
		out, path, err := fn(ctx, key)
		profileWrite(map[string]any{"type": "get", "start": t.UnixNano(), "seconds": time.Since(t).Seconds(), "hit": out != "", "error": err != nil})
		return out, path, err
	}
}

// Keep filepath imported in both instrumented source configurations.
var _ = filepath.Separator
