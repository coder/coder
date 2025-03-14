package healthcheck
import (
	"errors"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"
	"github.com/coder/coder/v2/coderd/healthcheck/health"
	"github.com/coder/coder/v2/codersdk/healthsdk"
	"github.com/coder/websocket"
)
type WebsocketReport healthsdk.WebsocketReport
type WebsocketReportOptions struct {
	APIKey     string
	AccessURL  *url.URL
	HTTPClient *http.Client
	Dismissed bool
}
func (r *WebsocketReport) Run(ctx context.Context, opts *WebsocketReportOptions) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	r.Severity = health.SeverityOK
	r.Warnings = []health.Message{}
	r.Dismissed = opts.Dismissed
	u, err := opts.AccessURL.Parse("/api/v2/debug/ws")
	if err != nil {
		r.Error = convertError(fmt.Errorf("parse access url: %w", err))
		r.Severity = health.SeverityError
		return
	}
	if u.Scheme == "https" {
		u.Scheme = "wss"
	} else {
		u.Scheme = "ws"
	}
	//nolint:bodyclose // websocket package closes this for you
	c, res, err := websocket.Dial(ctx, u.String(), &websocket.DialOptions{
		HTTPClient: opts.HTTPClient,
		HTTPHeader: http.Header{"Coder-Session-Token": []string{opts.APIKey}},
	})
	if res != nil {
		var body string
		if res.Body != nil {
			b, err := io.ReadAll(res.Body)
			if err == nil {
				body = string(b)
			}
		}
		r.Body = body
		r.Code = res.StatusCode
	}
	if err != nil {
		r.Error = convertError(fmt.Errorf("websocket dial: %w", err))
		r.Error = health.Errorf(health.CodeWebsocketDial, "websocket dial: %s", err)
		r.Severity = health.SeverityError
		return
	}
	defer c.Close(websocket.StatusGoingAway, "goodbye")
	for i := 0; i < 3; i++ {
		msg := strconv.Itoa(i)
		err := c.Write(ctx, websocket.MessageText, []byte(msg))
		if err != nil {
			r.Error = health.Errorf(health.CodeWebsocketEcho, "write message: %s", err)
			r.Severity = health.SeverityError
			return
		}
		ty, got, err := c.Read(ctx)
		if err != nil {
			r.Error = health.Errorf(health.CodeWebsocketEcho, "read message: %s", err)
			r.Severity = health.SeverityError
			return
		}
		if ty != websocket.MessageText {
			r.Error = health.Errorf(health.CodeWebsocketMsg, "received incorrect message type: %v", ty)
			r.Severity = health.SeverityError
			return
		}
		if string(got) != msg {
			r.Error = health.Errorf(health.CodeWebsocketMsg, "received incorrect message: wanted %q, got %q", msg, string(got))
			r.Severity = health.SeverityError
			return
		}
	}
	c.Close(websocket.StatusGoingAway, "goodbye")
	r.Healthy = true
}
type WebsocketEchoServer struct {
	Error error
	Code  int
}
func (s *WebsocketEchoServer) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	if s.Error != nil {
		rw.WriteHeader(s.Code)
		_, _ = rw.Write([]byte(s.Error.Error()))
		return
	}
	ctx := r.Context()
	c, err := websocket.Accept(rw, r, &websocket.AcceptOptions{})
	if err != nil {
		rw.WriteHeader(http.StatusBadRequest)
		_, _ = rw.Write([]byte(fmt.Sprint("unable to accept:", err)))
		return
	}
	defer c.Close(websocket.StatusGoingAway, "goodbye")
	echo := func() error {
		ctx, cancel := context.WithTimeout(ctx, time.Second*10)
		defer cancel()
		typ, r, err := c.Reader(ctx)
		if err != nil {
			return fmt.Errorf("get reader: %w", err)
		}
		w, err := c.Writer(ctx, typ)
		if err != nil {
			return fmt.Errorf("get writer: %w", err)
		}
		_, err = io.Copy(w, r)
		if err != nil {
			return fmt.Errorf("echo message: %w", err)
		}
		err = w.Close()
		return err
	}
	for {
		err := echo()
		if err != nil {
			return
		}
	}
}
