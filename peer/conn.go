package peer

import (
	"bytes"
	"context"
	"crypto/rand"
	"io"
	"sync"
	"time"

	"github.com/pion/logging"
	"github.com/pion/webrtc/v3"
	"go.uber.org/atomic"
	"golang.org/x/xerrors"

	"cdr.dev/slog"
)

var (
	// ErrDisconnected occurs when the connection has disconnected.
	// The connection will be attempting to reconnect at this point.
	ErrDisconnected = xerrors.New("connection is disconnected")
	// ErrFailed occurs when the connection has failed.
	// The connection will not retry after this point.
	ErrFailed = xerrors.New("connection has failed")
	// ErrClosed occurs when the connection was closed. It wraps io.EOF
	// to fulfill expected read errors from closed pipes.
	ErrClosed = xerrors.Errorf("connection was closed: %w", io.EOF)

	// The amount of random bytes sent in a ping.
	pingDataLength = 64
)

// Client creates a new client connection.
func Client(servers []webrtc.ICEServer, opts *ConnOpts) (*Conn, error) {
	return newWithClientOrServer(servers, true, opts)
}

// Server creates a new server connection.
func Server(servers []webrtc.ICEServer, opts *ConnOpts) (*Conn, error) {
	return newWithClientOrServer(servers, false, opts)
}

// newWithClientOrServer constructs a new connection with the client option.
// nolint:revive
func newWithClientOrServer(servers []webrtc.ICEServer, client bool, opts *ConnOpts) (*Conn, error) {
	if opts == nil {
		opts = &ConnOpts{}
	}

	// Enables preference to STUN.
	opts.SettingEngine.SetSrflxAcceptanceMinWait(0)
	opts.SettingEngine.DetachDataChannels()
	lf := logging.NewDefaultLoggerFactory()
	lf.DefaultLogLevel = logging.LogLevelDisabled
	opts.SettingEngine.LoggerFactory = lf
	api := webrtc.NewAPI(webrtc.WithSettingEngine(opts.SettingEngine))
	rtc, err := api.NewPeerConnection(webrtc.Configuration{
		ICEServers: servers,
	})
	if err != nil {
		return nil, xerrors.Errorf("create peer connection: %w", err)
	}
	conn := &Conn{
		pingChannelID:                   1,
		pingEchoChannelID:               2,
		opts:                            opts,
		rtc:                             rtc,
		offerrer:                        client,
		closed:                          make(chan struct{}),
		dcOpenChannel:                   make(chan *webrtc.DataChannel),
		dcDisconnectChannel:             make(chan struct{}),
		dcFailedChannel:                 make(chan struct{}),
		localCandidateChannel:           make(chan webrtc.ICECandidateInit),
		localSessionDescriptionChannel:  make(chan webrtc.SessionDescription),
		remoteSessionDescriptionChannel: make(chan webrtc.SessionDescription),
	}
	if client {
		// If we're the client, we want to flip the echo and
		// ping channel IDs so pings don't accidentally hit each other.
		conn.pingChannelID, conn.pingEchoChannelID = conn.pingEchoChannelID, conn.pingChannelID
	}
	err = conn.init()
	if err != nil {
		return nil, xerrors.Errorf("init: %w", err)
	}
	return conn, nil
}

type ConnOpts struct {
	Logger slog.Logger

	// Enables customization on the underlying WebRTC connection.
	SettingEngine webrtc.SettingEngine
}

// Conn represents a WebRTC peer connection.
//
// This struct wraps webrtc.PeerConnection to add bidirectional pings,
// concurrent-safe webrtc.DataChannel, and standardized errors for connection state.
type Conn struct {
	rtc  *webrtc.PeerConnection
	opts *ConnOpts
	// Determines whether this connection will send the offer or the answer.
	offerrer bool

	closed     chan struct{}
	closeMutex sync.Mutex
	closeError error

	dcOpenChannel         chan *webrtc.DataChannel
	dcDisconnectChannel   chan struct{}
	dcDisconnectListeners atomic.Uint32
	dcFailedChannel       chan struct{}
	dcFailedListeners     atomic.Uint32
	dcClosedWaitGroup     sync.WaitGroup

	localCandidateChannel           chan webrtc.ICECandidateInit
	localSessionDescriptionChannel  chan webrtc.SessionDescription
	remoteSessionDescriptionChannel chan webrtc.SessionDescription
	remoteSessionDescriptionMutex   sync.Mutex

	pingChannelID     uint16
	pingEchoChannelID uint16

	pingEchoChan  *Channel
	pingEchoOnce  sync.Once
	pingEchoError error
	pingMutex     sync.Mutex
	pingOnce      sync.Once
	pingChan      *Channel
	pingError     error
}

func (c *Conn) init() error {
	c.rtc.OnNegotiationNeeded(c.negotiate)
	c.rtc.OnDataChannel(func(dc *webrtc.DataChannel) {
		select {
		case <-c.closed:
			return
		case c.dcOpenChannel <- dc:
		default:
		}
	})
	c.rtc.OnConnectionStateChange(func(pcs webrtc.PeerConnectionState) {
		// Close must be locked here otherwise log output can appear
		// after the connection has been closed.
		c.closeMutex.Lock()
		defer c.closeMutex.Unlock()
		if c.isClosed() {
			return
		}

		c.opts.Logger.Debug(context.Background(), "rtc connection updated",
			slog.F("state", pcs),
			slog.F("ice", c.rtc.ICEConnectionState()))

		switch pcs {
		case webrtc.PeerConnectionStateDisconnected:
			for i := 0; i < int(c.dcDisconnectListeners.Load()); i++ {
				select {
				case c.dcDisconnectChannel <- struct{}{}:
				default:
				}
			}
		case webrtc.PeerConnectionStateFailed:
			for i := 0; i < int(c.dcFailedListeners.Load()); i++ {
				select {
				case c.dcFailedChannel <- struct{}{}:
				default:
				}
			}
		}
	})
	_, err := c.pingChannel()
	if err != nil {
		return err
	}
	_, err = c.pingEchoChannel()
	if err != nil {
		return err
	}

	return nil
}

func (c *Conn) pingChannel() (*Channel, error) {
	c.pingOnce.Do(func() {
		c.pingChan, c.pingError = c.dialChannel(context.Background(), "ping", &ChannelOpts{
			ID:               c.pingChannelID,
			Negotiated:       true,
			OpenOnDisconnect: true,
		})
		if c.pingError != nil {
			return
		}
	})
	return c.pingChan, c.pingError
}

func (c *Conn) pingEchoChannel() (*Channel, error) {
	c.pingEchoOnce.Do(func() {
		c.pingEchoChan, c.pingEchoError = c.dialChannel(context.Background(), "echo", &ChannelOpts{
			ID:               c.pingEchoChannelID,
			Negotiated:       true,
			OpenOnDisconnect: true,
		})
		if c.pingEchoError != nil {
			return
		}
		go func() {
			for {
				data := make([]byte, pingDataLength)
				bytesRead, err := c.pingEchoChan.Read(data)
				if err != nil {
					if c.isClosed() {
						return
					}
					_ = c.CloseWithError(xerrors.Errorf("read ping echo channel: %w", err))
					return
				}
				_, err = c.pingEchoChan.Write(data[:bytesRead])
				if err != nil {
					_ = c.CloseWithError(xerrors.Errorf("write ping echo channel: %w", err))
					return
				}
			}
		}()
	})
	return c.pingEchoChan, c.pingEchoError
}

func (c *Conn) negotiate() {
	c.opts.Logger.Debug(context.Background(), "negotiating")
	flushCandidates := c.proxyICECandidates()

	// Locks while the negotiation for a remote session
	// description is taking place.
	c.remoteSessionDescriptionMutex.Lock()
	defer c.remoteSessionDescriptionMutex.Unlock()

	if c.offerrer {
		offer, err := c.rtc.CreateOffer(&webrtc.OfferOptions{})
		if err != nil {
			_ = c.CloseWithError(xerrors.Errorf("create offer: %w", err))
			return
		}
		err = c.rtc.SetLocalDescription(offer)
		if err != nil {
			_ = c.CloseWithError(xerrors.Errorf("set local description: %w", err))
			return
		}
		select {
		case <-c.closed:
			return
		case c.localSessionDescriptionChannel <- offer:
		}
	}

	var remoteDescription webrtc.SessionDescription
	select {
	case <-c.closed:
		return
	case remoteDescription = <-c.remoteSessionDescriptionChannel:
	}

	err := c.rtc.SetRemoteDescription(remoteDescription)
	if err != nil {
		_ = c.CloseWithError(xerrors.Errorf("set remote description (closed %v): %w", c.isClosed(), err))
		return
	}

	if !c.offerrer {
		answer, err := c.rtc.CreateAnswer(&webrtc.AnswerOptions{})
		if err != nil {
			_ = c.CloseWithError(xerrors.Errorf("create answer: %w", err))
			return
		}
		err = c.rtc.SetLocalDescription(answer)
		if err != nil {
			_ = c.CloseWithError(xerrors.Errorf("set local description: %w", err))
			return
		}
		if c.isClosed() {
			return
		}
		select {
		case <-c.closed:
			return
		case c.localSessionDescriptionChannel <- answer:
		}
	}

	flushCandidates()
	c.opts.Logger.Debug(context.Background(), "flushed candidates")
}

func (c *Conn) proxyICECandidates() func() {
	var (
		mut     sync.Mutex
		queue   = []webrtc.ICECandidateInit{}
		flushed = false
	)
	c.rtc.OnICECandidate(func(iceCandidate *webrtc.ICECandidate) {
		if iceCandidate == nil {
			return
		}
		mut.Lock()
		defer mut.Unlock()
		if !flushed {
			queue = append(queue, iceCandidate.ToJSON())
			return
		}
		select {
		case <-c.closed:
			return
		case c.localCandidateChannel <- iceCandidate.ToJSON():
		}
	})
	return func() {
		mut.Lock()
		defer mut.Unlock()
		for _, q := range queue {
			select {
			case <-c.closed:
				break
			case c.localCandidateChannel <- q:
			}
		}
		flushed = true
	}
}

// LocalCandidate returns a channel that emits when a local candidate
// needs to be exchanged with a remote connection.
func (c *Conn) LocalCandidate() <-chan webrtc.ICECandidateInit {
	return c.localCandidateChannel
}

// AddRemoteCandidate adds a remote candidate to the RTC connection.
func (c *Conn) AddRemoteCandidate(i webrtc.ICECandidateInit) error {
	// Prevents candidates from being added before an offer<->answer has occurred.
	c.remoteSessionDescriptionMutex.Lock()
	defer c.remoteSessionDescriptionMutex.Unlock()
	return c.rtc.AddICECandidate(i)
}

// LocalSessionDescription returns a channel that emits a session description
// when one is required to be exchanged.
func (c *Conn) LocalSessionDescription() <-chan webrtc.SessionDescription {
	return c.localSessionDescriptionChannel
}

// SetConfiguration applies options to the WebRTC connection.
// Generally used for updating transport options, like ICE servers.
func (c *Conn) SetConfiguration(configuration webrtc.Configuration) error {
	return c.rtc.SetConfiguration(configuration)
}

// SetRemoteSessionDescription sets the remote description for the WebRTC connection.
func (c *Conn) SetRemoteSessionDescription(sessionDescription webrtc.SessionDescription) {
	if c.isClosed() {
		return
	}
	c.closeMutex.Lock()
	defer c.closeMutex.Unlock()
	select {
	case <-c.closed:
	case c.remoteSessionDescriptionChannel <- sessionDescription:
	}
}

// Accept blocks waiting for a channel to be opened.
func (c *Conn) Accept(ctx context.Context) (*Channel, error) {
	var dataChannel *webrtc.DataChannel
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-c.closed:
		return nil, c.closeError
	case dataChannel = <-c.dcOpenChannel:
	}

	return newChannel(c, dataChannel, &ChannelOpts{}), nil
}

// Dial creates a new DataChannel.
func (c *Conn) Dial(ctx context.Context, label string, opts *ChannelOpts) (*Channel, error) {
	if opts == nil {
		opts = &ChannelOpts{}
	}
	if opts.ID == c.pingChannelID || opts.ID == c.pingEchoChannelID {
		return nil, xerrors.Errorf("datachannel id %d and %d are reserved for ping", c.pingChannelID, c.pingEchoChannelID)
	}
	return c.dialChannel(ctx, label, opts)
}

func (c *Conn) dialChannel(ctx context.Context, label string, opts *ChannelOpts) (*Channel, error) {
	c.opts.Logger.Debug(ctx, "creating data channel", slog.F("label", label), slog.F("opts", opts))
	var id *uint16
	if opts.ID != 0 {
		id = &opts.ID
	}
	ordered := true
	if opts.Unordered {
		ordered = false
	}
	if opts.OpenOnDisconnect && !opts.Negotiated {
		return nil, xerrors.New("OpenOnDisconnect is only allowed for Negotiated channels")
	}
	if c.isClosed() {
		return nil, xerrors.Errorf("closed: %w", c.closeError)
	}

	dataChannel, err := c.rtc.CreateDataChannel(label, &webrtc.DataChannelInit{
		ID:         id,
		Negotiated: &opts.Negotiated,
		Ordered:    &ordered,
		Protocol:   &opts.Protocol,
	})
	if err != nil {
		return nil, xerrors.Errorf("create data channel: %w", err)
	}
	return newChannel(c, dataChannel, opts), nil
}

// Ping returns the duration it took to round-trip data.
// Multiple pings cannot occur at the same time, so this function will block.
func (c *Conn) Ping() (time.Duration, error) {
	// Pings are not async, so we need a mutex.
	c.pingMutex.Lock()
	defer c.pingMutex.Unlock()

	ping, err := c.pingChannel()
	if err != nil {
		return 0, xerrors.Errorf("get ping channel: %w", err)
	}
	pingDataSent := make([]byte, pingDataLength)
	_, err = rand.Read(pingDataSent)
	if err != nil {
		return 0, xerrors.Errorf("read random ping data: %w", err)
	}
	start := time.Now()
	_, err = ping.Write(pingDataSent)
	if err != nil {
		return 0, xerrors.Errorf("send ping: %w", err)
	}
	c.opts.Logger.Debug(context.Background(), "wrote ping",
		slog.F("connection_state", c.rtc.ConnectionState()))

	pingDataReceived := make([]byte, pingDataLength)
	_, err = ping.Read(pingDataReceived)
	if err != nil {
		return 0, xerrors.Errorf("read ping: %w", err)
	}
	end := time.Now()
	if !bytes.Equal(pingDataSent, pingDataReceived) {
		return 0, xerrors.Errorf("ping data inconsistency sent != received")
	}
	return end.Sub(start), nil
}

func (c *Conn) Closed() <-chan struct{} {
	return c.closed
}

// Close closes the connection and frees all associated resources.
func (c *Conn) Close() error {
	return c.CloseWithError(nil)
}

func (c *Conn) isClosed() bool {
	select {
	case <-c.closed:
		return true
	default:
		return false
	}
}

// CloseWithError closes the connection; subsequent reads/writes will return the error err.
func (c *Conn) CloseWithError(err error) error {
	c.closeMutex.Lock()
	defer c.closeMutex.Unlock()

	if c.isClosed() {
		return c.closeError
	}

	c.opts.Logger.Debug(context.Background(), "closing conn with error", slog.Error(err))
	if err == nil {
		c.closeError = ErrClosed
	} else {
		c.closeError = err
	}
	close(c.closed)

	if ch, _ := c.pingChannel(); ch != nil {
		_ = ch.closeWithError(c.closeError)
	}
	// If the WebRTC connection has already been closed (due to failure or disconnect),
	// this call will return an error that isn't typed. We don't check the error because
	// closing an already closed connection isn't an issue for us.
	_ = c.rtc.Close()

	// Waits for all DataChannels to exit before officially labeling as closed.
	// All logging, goroutines, and async functionality is cleaned up after this.
	c.dcClosedWaitGroup.Wait()

	return err
}
