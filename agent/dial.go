package agent

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"net/url"
	"strings"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/peer"
)

// DialResponse is written to datachannels with protocol "dial" by the agent as
// the first packet to signify whether the dial succeeded or failed.
type DialResponse struct {
	Error string `json:"error,omitempty"`
}

// Dial dials an arbitrary protocol+address from inside the workspace and
// proxies it through the provided net.Conn.
func (c *Conn) Dial(network string, addr string) (net.Conn, error) {
	// Force unique URL by including a random UUID.
	id, err := uuid.NewRandom()
	if err != nil {
		return nil, xerrors.Errorf("generate random UUID: %w", err)
	}

	host := ""
	path := ""
	if strings.HasPrefix(network, "unix") {
		path = addr
	} else {
		host = addr
	}

	label := (&url.URL{
		Scheme: network,
		Host:   host,
		Path:   path,
		RawQuery: (url.Values{
			"id": []string{id.String()},
		}).Encode(),
	}).String()

	channel, err := c.OpenChannel(context.Background(), label, &peer.ChannelOptions{
		Protocol: "dial",
	})
	if err != nil {
		return nil, xerrors.Errorf("pty: %w", err)
	}

	// The first message written from the other side is a JSON payload
	// containing the dial error.
	dec := json.NewDecoder(channel)
	var res DialResponse
	err = dec.Decode(&res)
	if err != nil {
		return nil, xerrors.Errorf("failed to decode initial packet: %w", err)
	}
	if res.Error != "" {
		_ = channel.Close()
		return nil, xerrors.Errorf("remote dial error: %v", res.Error)
	}

	return channel.NetConn(), nil
}

func (*agent) handleDial(ctx context.Context, label string, conn net.Conn) {
	defer conn.Close()

	writeError := func(responseError error) error {
		msg := ""
		if responseError != nil {
			msg = responseError.Error()
		}
		b, err := json.Marshal(DialResponse{
			Error: msg,
		})
		if err != nil {
			return xerrors.Errorf("marshal agent webrtc dial response: %w", err)
		}

		_, err = conn.Write(b)
		return err
	}

	u, err := url.Parse(label)
	if err != nil {
		_ = writeError(xerrors.Errorf("parse URL %q: %w", label, err))
		return
	}

	network := u.Scheme
	addr := u.Host + u.Path
	nconn, err := net.Dial(network, addr)
	if err != nil {
		_ = writeError(xerrors.Errorf("dial '%v://%v': %w", network, addr, err))
		return
	}

	err = writeError(nil)
	if err != nil {
		return
	}

	bicopy(ctx, conn, nconn)
}

// bicopy copies all of the data between the two connections
// and will close them after one or both of them are done writing.
// If the context is canceled, both of the connections will be
// closed.
//
// NOTE: This function will block until the copying is done or the
// context is canceled.
func bicopy(ctx context.Context, c1, c2 io.ReadWriteCloser) {
	defer c1.Close()
	defer c2.Close()

	ctx, cancel := context.WithCancel(ctx)

	copyFunc := func(dst io.WriteCloser, src io.Reader) {
		defer cancel()
		_, _ = io.Copy(dst, src)
	}

	go copyFunc(c1, c2)
	go copyFunc(c2, c1)

	<-ctx.Done()
}
