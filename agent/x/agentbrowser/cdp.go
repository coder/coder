package agentbrowser

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"sync"

	"golang.org/x/xerrors"
)

// cdpConn speaks the Chrome DevTools Protocol over the pipe Chrome opens
// with --remote-debugging-pipe: NUL-delimited JSON messages in each
// direction. Using the pipe instead of a TCP debugging port means no other
// process in the workspace can attach to the browser.
type cdpConn struct {
	w io.WriteCloser
	r io.ReadCloser

	writeMu sync.Mutex

	mu      sync.Mutex
	nextID  int64
	pending map[int64]chan cdpMessage
	err     error

	events chan cdpMessage
	done   chan struct{}
}

type cdpMessage struct {
	ID        int64           `json:"id,omitempty"`
	SessionID string          `json:"sessionId,omitempty"`
	Method    string          `json:"method,omitempty"`
	Params    json.RawMessage `json:"params,omitempty"`
	Result    json.RawMessage `json:"result,omitempty"`
	Error     *cdpError       `json:"error,omitempty"`
}

type cdpError struct {
	Code    int64  `json:"code"`
	Message string `json:"message"`
}

func (e *cdpError) Error() string {
	return xerrors.Errorf("cdp error %d: %s", e.Code, e.Message).Error()
}

// newCDPConn starts reading from r. Events are delivered in order to
// handle on a dedicated goroutine so handlers may issue calls.
func newCDPConn(w io.WriteCloser, r io.ReadCloser, handle func(cdpMessage)) *cdpConn {
	c := &cdpConn{
		w:       w,
		r:       r,
		pending: make(map[int64]chan cdpMessage),
		events:  make(chan cdpMessage, 1024),
		done:    make(chan struct{}),
	}
	go c.readLoop()
	go func() {
		for ev := range c.events {
			handle(ev)
		}
	}()
	return c
}

func (c *cdpConn) readLoop() {
	defer close(c.events)
	br := bufio.NewReaderSize(c.r, 1<<20)
	for {
		line, err := br.ReadBytes(0)
		if err != nil {
			c.fail(xerrors.Errorf("read cdp pipe: %w", err))
			return
		}
		var msg cdpMessage
		if err := json.Unmarshal(line[:len(line)-1], &msg); err != nil {
			continue
		}
		if msg.ID != 0 {
			c.mu.Lock()
			ch, ok := c.pending[msg.ID]
			delete(c.pending, msg.ID)
			c.mu.Unlock()
			if ok {
				ch <- msg
			}
			continue
		}
		select {
		case c.events <- msg:
		default:
			// Drop events rather than stall responses; state is
			// re-derived from the next event that gets through.
		}
	}
}

func (c *cdpConn) fail(err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.err != nil {
		return
	}
	c.err = err
	close(c.done)
	for id, ch := range c.pending {
		close(ch)
		delete(c.pending, id)
	}
}

// Call sends method with params to the target identified by sessionID
// ("" for the browser) and decodes the result into result if non-nil.
func (c *cdpConn) Call(ctx context.Context, sessionID, method string, params, result any) error {
	c.mu.Lock()
	if c.err != nil {
		err := c.err
		c.mu.Unlock()
		return err
	}
	c.nextID++
	id := c.nextID
	ch := make(chan cdpMessage, 1)
	c.pending[id] = ch
	c.mu.Unlock()

	msg := cdpMessage{ID: id, SessionID: sessionID, Method: method}
	if params != nil {
		raw, err := json.Marshal(params)
		if err != nil {
			return xerrors.Errorf("marshal %s params: %w", method, err)
		}
		msg.Params = raw
	}
	data, err := json.Marshal(msg)
	if err != nil {
		return xerrors.Errorf("marshal %s: %w", method, err)
	}
	c.writeMu.Lock()
	_, err = c.w.Write(append(data, 0))
	c.writeMu.Unlock()
	if err != nil {
		c.fail(xerrors.Errorf("write cdp pipe: %w", err))
		return xerrors.Errorf("write %s: %w", method, err)
	}

	select {
	case <-ctx.Done():
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
		return ctx.Err()
	case resp, ok := <-ch:
		if !ok {
			c.mu.Lock()
			err := c.err
			c.mu.Unlock()
			return err
		}
		if resp.Error != nil {
			return xerrors.Errorf("%s: %w", method, resp.Error)
		}
		if result != nil && len(resp.Result) > 0 {
			if err := json.Unmarshal(resp.Result, result); err != nil {
				return xerrors.Errorf("decode %s result: %w", method, err)
			}
		}
		return nil
	}
}

// Done is closed when the pipe fails or is closed.
func (c *cdpConn) Done() <-chan struct{} { return c.done }

func (c *cdpConn) Close() error {
	c.fail(xerrors.New("cdp connection closed"))
	_ = c.w.Close()
	return c.r.Close()
}
