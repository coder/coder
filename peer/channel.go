package peer

import (
	"bufio"
	"context"
	"io"
	"net"
	"sync"

	"github.com/pion/datachannel"
	"github.com/pion/webrtc/v3"
	"golang.org/x/xerrors"

	"cdr.dev/slog"
)

const (
	bufferedAmountLowThreshold uint64 = 512 * 1024  // 512 KB
	maxBufferedAmount          uint64 = 1024 * 1024 // 1 MB
	// For some reason messages larger just don't work...
	// This shouldn't be a huge deal for real-world usage.
	// See: https://github.com/pion/datachannel/issues/59
	maxMessageLength = 64 * 1024 // 64 KB
)

// newChannel creates a new channel and initializes it.
// The initialization overrides listener handles, and detaches
// the channel on open. The datachannel should not be manually
// mutated after being passed to this function.
func newChannel(conn *Conn, dc *webrtc.DataChannel, opts *ChannelOptions) *Channel {
	channel := &Channel{
		opts: opts,
		conn: conn,
		dc:   dc,

		opened:   make(chan struct{}),
		closed:   make(chan struct{}),
		sendMore: make(chan struct{}, 1),
	}
	channel.init()
	return channel
}

type ChannelOptions struct {
	// ID is a channel ID that should be used when `Negotiated`
	// is true.
	ID uint16

	// Negotiated returns whether the data channel will already
	// be active on the other end. Defaults to false.
	Negotiated bool

	// Arbitrary string that can be parsed on `Accept`.
	Protocol string

	// Unordered determines whether the channel acts like
	// a UDP connection. Defaults to false.
	Unordered bool

	// Whether the channel will be left open on disconnect or not.
	// If true, data will be buffered on either end to be sent
	// once reconnected. Defaults to false.
	OpenOnDisconnect bool
}

// Channel represents a WebRTC DataChannel.
//
// This struct wraps webrtc.DataChannel to add concurrent-safe usage,
// data bufferring, and standardized errors for connection state.
//
// It modifies the default behavior of a DataChannel by closing on
// WebRTC PeerConnection failure. This is done to emulate TCP connections.
// This option can be changed in the options when creating a Channel.
type Channel struct {
	opts *ChannelOptions

	conn *Conn
	dc   *webrtc.DataChannel
	// This field can be nil. It becomes set after the DataChannel
	// has been opened and is detached.
	rwc    datachannel.ReadWriteCloser
	reader io.Reader

	closed     chan struct{}
	closeMutex sync.Mutex
	closeError error

	opened chan struct{}

	// sendMore is used to block Write operations on a full buffer.
	// It's signaled when the buffer can accept more data.
	sendMore   chan struct{}
	writeMutex sync.Mutex
}

// init attaches listeners to the DataChannel to detect opening,
// closing, and when the channel is ready to transmit data.
//
// This should only be called once on creation.
func (c *Channel) init() {
	// WebRTC connections maintain an internal buffer that can fill when:
	// 1. Data is being sent faster than it can flush.
	// 2. The connection is disconnected, but data is still being sent.
	//
	// This applies a maximum in-memory buffer for data, and will cause
	// write operations to block once the threshold is set.
	c.dc.SetBufferedAmountLowThreshold(bufferedAmountLowThreshold)
	c.dc.OnBufferedAmountLow(func() {
		if c.isClosed() {
			return
		}
		select {
		case <-c.closed:
			return
		case c.sendMore <- struct{}{}:
		default:
		}
	})
	c.dc.OnClose(func() {
		c.conn.logger().Debug(context.Background(), "datachannel closing from OnClose", slog.F("id", c.dc.ID()), slog.F("label", c.dc.Label()))
		_ = c.closeWithError(ErrClosed)
	})
	c.dc.OnOpen(func() {
		c.closeMutex.Lock()
		defer c.closeMutex.Unlock()

		c.conn.logger().Debug(context.Background(), "datachannel opening", slog.F("id", c.dc.ID()), slog.F("label", c.dc.Label()))
		var err error
		c.rwc, err = c.dc.Detach()
		if err != nil {
			_ = c.closeWithError(xerrors.Errorf("detach: %w", err))
			return
		}
		// pion/webrtc will return an io.ErrShortBuffer when a read
		// is triggerred with a buffer size less than the chunks written.
		//
		// This makes sense when considering UDP connections, because
		// bufferring of data that has no transmit guarantees is likely
		// to cause unexpected behavior.
		//
		// When ordered, this adds a bufio.Reader. This ensures additional
		// data on TCP-like connections can be read in parts, while still
		// being bufferred.
		if c.opts.Unordered {
			c.reader = c.rwc
		} else {
			// This must be the max message length otherwise a short
			// buffer error can occur.
			c.reader = bufio.NewReaderSize(c.rwc, maxMessageLength)
		}
		close(c.opened)
	})

	c.conn.dcDisconnectListeners.Add(1)
	c.conn.dcFailedListeners.Add(1)
	c.conn.dcClosedWaitGroup.Add(1)
	go func() {
		var err error
		// A DataChannel can disconnect multiple times, so this needs to loop.
		for {
			select {
			case <-c.conn.closedRTC:
				// If this channel was closed, there's no need to close again.
				err = c.conn.closeError
			case <-c.conn.Closed():
				// If the RTC connection closed with an error, this channel
				// should end with the same one.
				err = c.conn.closeError
			case <-c.conn.dcDisconnectChannel:
				// If the RTC connection is disconnected, we need to check if
				// the DataChannel is supposed to end on disconnect.
				if c.opts.OpenOnDisconnect {
					continue
				}
				err = xerrors.Errorf("rtc disconnected. closing: %w", ErrClosed)
			case <-c.conn.dcFailedChannel:
				// If the RTC connection failed, close the Channel.
				err = ErrFailed
			}
			if err != nil {
				break
			}
		}
		_ = c.closeWithError(err)
	}()
}

// Read blocks until data is received.
//
// This will block until the underlying DataChannel has been opened.
func (c *Channel) Read(bytes []byte) (int, error) {
	if c.isClosed() {
		return 0, c.closeError
	}
	if !c.isOpened() {
		err := c.waitOpened()
		if err != nil {
			return 0, err
		}
	}

	bytesRead, err := c.reader.Read(bytes)
	if err != nil {
		if c.isClosed() {
			return 0, c.closeError
		}
		// An EOF always occurs when the connection is closed.
		// Alternative close errors will occur first if an unexpected
		// close has occurred.
		if xerrors.Is(err, io.EOF) {
			err = c.closeWithError(ErrClosed)
		}
		return bytesRead, err
	}
	return bytesRead, err
}

// Write sends data to the underlying DataChannel.
//
// This function will block if too much data is being sent.
// Data will buffer if the connection is temporarily disconnected,
// and will be flushed upon reconnection.
//
// If the Channel is setup to close on disconnect, any buffered
// data will be lost.
func (c *Channel) Write(bytes []byte) (n int, err error) {
	if len(bytes) > maxMessageLength {
		return 0, xerrors.Errorf("outbound packet larger than maximum message size: %d", maxMessageLength)
	}

	c.writeMutex.Lock()
	defer c.writeMutex.Unlock()

	if c.isClosed() {
		return 0, c.closeWithError(nil)
	}
	if !c.isOpened() {
		err := c.waitOpened()
		if err != nil {
			return 0, err
		}
	}

	if c.dc.BufferedAmount()+uint64(len(bytes)) >= maxBufferedAmount {
		<-c.sendMore
	}
	return c.rwc.Write(bytes)
}

// Close gracefully closes the DataChannel.
func (c *Channel) Close() error {
	return c.closeWithError(nil)
}

// Label returns the label of the underlying DataChannel.
func (c *Channel) Label() string {
	return c.dc.Label()
}

// Protocol returns the protocol of the underlying DataChannel.
func (c *Channel) Protocol() string {
	return c.dc.Protocol()
}

// NetConn wraps the DataChannel in a struct fulfilling net.Conn.
// Read, Write, and Close operations can still be used on the *Channel struct.
func (c *Channel) NetConn() net.Conn {
	return &fakeNetConn{
		c:    c,
		addr: &peerAddr{},
	}
}

// closeWithError closes the Channel with the error provided.
// If a graceful close occurs, the error will be nil.
func (c *Channel) closeWithError(err error) error {
	c.closeMutex.Lock()
	defer c.closeMutex.Unlock()

	if c.isClosed() {
		return c.closeError
	}

	c.conn.logger().Debug(context.Background(), "datachannel closing with error", slog.F("id", c.dc.ID()), slog.F("label", c.dc.Label()), slog.Error(err))
	if err == nil {
		c.closeError = ErrClosed
	} else {
		c.closeError = err
	}
	if c.rwc != nil {
		_ = c.rwc.Close()
	}
	_ = c.dc.Close()

	close(c.closed)
	close(c.sendMore)
	c.conn.dcDisconnectListeners.Sub(1)
	c.conn.dcFailedListeners.Sub(1)
	c.conn.dcClosedWaitGroup.Done()

	return err
}

func (c *Channel) isClosed() bool {
	select {
	case <-c.closed:
		return true
	default:
		return false
	}
}

func (c *Channel) isOpened() bool {
	select {
	case <-c.opened:
		return true
	default:
		return false
	}
}

func (c *Channel) waitOpened() error {
	select {
	case <-c.opened:
		return nil
	case <-c.closed:
		return c.closeError
	}
}
