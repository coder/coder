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
func Client(servers []webrtc.ICEServer, opts *ConnOptions) (*Conn, error) {
	return newWithClientOrServer(servers, true, opts)
}

// Server creates a new server connection.
func Server(servers []webrtc.ICEServer, opts *ConnOptions) (*Conn, error) {
	return newWithClientOrServer(servers, false, opts)
}

// newWithClientOrServer constructs a new connection with the client option.
// nolint:revive
func newWithClientOrServer(servers []webrtc.ICEServer, client bool, opts *ConnOptions) (*Conn, error) {
	if opts == nil {
		opts = &ConnOptions{}
	}

	opts.SettingEngine.DetachDataChannels()
	logger := logging.NewDefaultLoggerFactory()
	logger.DefaultLogLevel = logging.LogLevelDisabled
	opts.SettingEngine.LoggerFactory = logger
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
		offerer:                         client,
		closed:                          make(chan struct{}),
		closedRTC:                       make(chan struct{}),
		closedICE:                       make(chan struct{}),
		dcOpenChannel:                   make(chan *webrtc.DataChannel),
		dcDisconnectChannel:             make(chan struct{}),
		dcFailedChannel:                 make(chan struct{}),
		localCandidateChannel:           make(chan webrtc.ICECandidateInit),
		localSessionDescriptionChannel:  make(chan webrtc.SessionDescription, 1),
		remoteSessionDescriptionChannel: make(chan webrtc.SessionDescription, 1),
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

type ConnOptions struct {
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
	opts *ConnOptions
	// Determines whether this connection will send the offer or the answer.
	offerer bool

	closed         chan struct{}
	closedRTC      chan struct{}
	closedRTCMutex sync.Mutex
	closedICE      chan struct{}
	closedICEMutex sync.Mutex
	closeMutex     sync.Mutex
	closeError     error

	dcOpenChannel         chan *webrtc.DataChannel
	dcDisconnectChannel   chan struct{}
	dcDisconnectListeners atomic.Uint32
	dcFailedChannel       chan struct{}
	dcFailedListeners     atomic.Uint32
	dcClosedWaitGroup     sync.WaitGroup

	localCandidateChannel           chan webrtc.ICECandidateInit
	localSessionDescriptionChannel  chan webrtc.SessionDescription
	remoteSessionDescriptionChannel chan webrtc.SessionDescription

	negotiateMutex sync.Mutex
	hasNegotiated  bool

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
	// The negotiation needed callback can take a little bit to execute!
	c.negotiateMutex.Lock()

	c.rtc.OnNegotiationNeeded(c.negotiate)
	c.rtc.OnICEConnectionStateChange(func(iceConnectionState webrtc.ICEConnectionState) {
		c.closedICEMutex.Lock()
		defer c.closedICEMutex.Unlock()
		select {
		case <-c.closedICE:
			// Don't log more state changes if we've already closed.
			return
		default:
			c.opts.Logger.Debug(context.Background(), "ice connection state updated",
				slog.F("state", iceConnectionState))

			if iceConnectionState == webrtc.ICEConnectionStateClosed {
				// pion/webrtc can update this state multiple times.
				// A connection can never become un-closed, so we
				// close the channel if it isn't already.
				close(c.closedICE)
			}
		}
	})
	c.rtc.OnICEGatheringStateChange(func(iceGatherState webrtc.ICEGathererState) {
		c.closedICEMutex.Lock()
		defer c.closedICEMutex.Unlock()
		select {
		case <-c.closedICE:
			// Don't log more state changes if we've already closed.
			return
		default:
			c.opts.Logger.Debug(context.Background(), "ice gathering state updated",
				slog.F("state", iceGatherState))

			if iceGatherState == webrtc.ICEGathererStateClosed {
				// pion/webrtc can update this state multiple times.
				// A connection can never become un-closed, so we
				// close the channel if it isn't already.
				close(c.closedICE)
			}
		}
	})
	c.rtc.OnConnectionStateChange(func(peerConnectionState webrtc.PeerConnectionState) {
		go func() {
			c.closeMutex.Lock()
			defer c.closeMutex.Unlock()
			if c.isClosed() {
				return
			}
			c.opts.Logger.Debug(context.Background(), "rtc connection updated",
				slog.F("state", peerConnectionState))
		}()

		switch peerConnectionState {
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
		case webrtc.PeerConnectionStateClosed:
			// pion/webrtc can update this state multiple times.
			// A connection can never become un-closed, so we
			// close the channel if it isn't already.
			c.closedRTCMutex.Lock()
			defer c.closedRTCMutex.Unlock()
			select {
			case <-c.closedRTC:
			default:
				close(c.closedRTC)
			}
		}
	})

	// These functions need to check if the conn is closed, because they can be
	// called after being closed.
	c.rtc.OnSignalingStateChange(func(signalState webrtc.SignalingState) {
		if c.isClosed() {
			return
		}
		c.opts.Logger.Debug(context.Background(), "signaling state updated",
			slog.F("state", signalState))
	})
	c.rtc.SCTP().Transport().OnStateChange(func(dtlsTransportState webrtc.DTLSTransportState) {
		if c.isClosed() {
			return
		}
		c.opts.Logger.Debug(context.Background(), "dtls transport state updated",
			slog.F("state", dtlsTransportState))
	})
	c.rtc.SCTP().Transport().ICETransport().OnSelectedCandidatePairChange(func(candidatePair *webrtc.ICECandidatePair) {
		if c.isClosed() {
			return
		}
		c.opts.Logger.Debug(context.Background(), "selected candidate pair changed",
			slog.F("local", candidatePair.Local), slog.F("remote", candidatePair.Remote))
	})
	c.rtc.OnICECandidate(func(iceCandidate *webrtc.ICECandidate) {
		if c.isClosed() {
			return
		}

		if iceCandidate == nil {
			return
		}
		// Run this in a goroutine so we don't block pion/webrtc
		// from continuing.
		go func() {
			c.opts.Logger.Debug(context.Background(), "sending local candidate", slog.F("candidate", iceCandidate.ToJSON().Candidate))
			select {
			case <-c.closed:
				break
			case c.localCandidateChannel <- iceCandidate.ToJSON():
			}
		}()
	})
	c.rtc.OnDataChannel(func(dc *webrtc.DataChannel) {
		select {
		case <-c.closed:
			return
		case c.dcOpenChannel <- dc:
		default:
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

// negotiate is triggered when a connection is ready to be established.
// See trickle ICE for the expected exchange: https://webrtchacks.com/trickle-ice/
func (c *Conn) negotiate() {
	c.opts.Logger.Debug(context.Background(), "negotiating")
	// ICE candidates cannot be added until SessionDescriptions have been
	// exchanged between peers.
	if c.hasNegotiated {
		c.negotiateMutex.Lock()
	}
	c.hasNegotiated = true
	defer c.negotiateMutex.Unlock()

	if c.offerer {
		offer, err := c.rtc.CreateOffer(&webrtc.OfferOptions{})
		if err != nil {
			_ = c.CloseWithError(xerrors.Errorf("create offer: %w", err))
			return
		}
		// pion/webrtc will panic if Close is called while this
		// function is being executed.
		c.closeMutex.Lock()
		err = c.rtc.SetLocalDescription(offer)
		c.closeMutex.Unlock()
		if err != nil {
			_ = c.CloseWithError(xerrors.Errorf("set local description: %w", err))
			return
		}
		c.opts.Logger.Debug(context.Background(), "sending offer", slog.F("offer", offer))
		select {
		case <-c.closed:
			return
		case c.localSessionDescriptionChannel <- offer:
		}
		c.opts.Logger.Debug(context.Background(), "sent offer")
	}

	var sessionDescription webrtc.SessionDescription
	c.opts.Logger.Debug(context.Background(), "awaiting remote description...")
	select {
	case <-c.closed:
		return
	case sessionDescription = <-c.remoteSessionDescriptionChannel:
	}
	c.opts.Logger.Debug(context.Background(), "setting remote description")

	err := c.rtc.SetRemoteDescription(sessionDescription)
	if err != nil {
		_ = c.CloseWithError(xerrors.Errorf("set remote description (closed %v): %w", c.isClosed(), err))
		return
	}

	if !c.offerer {
		answer, err := c.rtc.CreateAnswer(&webrtc.AnswerOptions{})
		if err != nil {
			_ = c.CloseWithError(xerrors.Errorf("create answer: %w", err))
			return
		}
		// pion/webrtc will panic if Close is called while this
		// function is being executed.
		c.closeMutex.Lock()
		err = c.rtc.SetLocalDescription(answer)
		c.closeMutex.Unlock()
		if err != nil {
			_ = c.CloseWithError(xerrors.Errorf("set local description: %w", err))
			return
		}
		c.opts.Logger.Debug(context.Background(), "sending answer", slog.F("answer", answer))
		select {
		case <-c.closed:
			return
		case c.localSessionDescriptionChannel <- answer:
		}
		c.opts.Logger.Debug(context.Background(), "sent answer")
	}
}

// AddRemoteCandidate adds a remote candidate to the RTC connection.
func (c *Conn) AddRemoteCandidate(i webrtc.ICECandidateInit) {
	if c.isClosed() {
		return
	}
	// This must occur in a goroutine to allow the SessionDescriptions
	// to be exchanged first.
	go func() {
		c.negotiateMutex.Lock()
		defer c.negotiateMutex.Unlock()
		if c.isClosed() {
			return
		}
		c.opts.Logger.Debug(context.Background(), "accepting candidate", slog.F("candidate", i.Candidate))
		err := c.rtc.AddICECandidate(i)
		if err != nil {
			if c.rtc.ConnectionState() == webrtc.PeerConnectionStateClosed {
				return
			}
			_ = c.CloseWithError(xerrors.Errorf("accept candidate: %w", err))
		}
	}()
}

// SetRemoteSessionDescription sets the remote description for the WebRTC connection.
func (c *Conn) SetRemoteSessionDescription(sessionDescription webrtc.SessionDescription) {
	select {
	case <-c.closed:
	case c.remoteSessionDescriptionChannel <- sessionDescription:
	}
}

// LocalSessionDescription returns a channel that emits a session description
// when one is required to be exchanged.
func (c *Conn) LocalSessionDescription() <-chan webrtc.SessionDescription {
	return c.localSessionDescriptionChannel
}

// LocalCandidate returns a channel that emits when a local candidate
// needs to be exchanged with a remote connection.
func (c *Conn) LocalCandidate() <-chan webrtc.ICECandidateInit {
	return c.localCandidateChannel
}

func (c *Conn) pingChannel() (*Channel, error) {
	c.pingOnce.Do(func() {
		c.pingChan, c.pingError = c.dialChannel(context.Background(), "ping", &ChannelOptions{
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
		c.pingEchoChan, c.pingEchoError = c.dialChannel(context.Background(), "echo", &ChannelOptions{
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

// SetConfiguration applies options to the WebRTC connection.
// Generally used for updating transport options, like ICE servers.
func (c *Conn) SetConfiguration(configuration webrtc.Configuration) error {
	return c.rtc.SetConfiguration(configuration)
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

	return newChannel(c, dataChannel, &ChannelOptions{}), nil
}

// Dial creates a new DataChannel.
func (c *Conn) Dial(ctx context.Context, label string, opts *ChannelOptions) (*Channel, error) {
	if opts == nil {
		opts = &ChannelOptions{}
	}
	if opts.ID == c.pingChannelID || opts.ID == c.pingEchoChannelID {
		return nil, xerrors.Errorf("datachannel id %d and %d are reserved for ping", c.pingChannelID, c.pingEchoChannelID)
	}
	return c.dialChannel(ctx, label, opts)
}

func (c *Conn) dialChannel(ctx context.Context, label string, opts *ChannelOptions) (*Channel, error) {
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

	if ch, _ := c.pingChannel(); ch != nil {
		_ = ch.closeWithError(c.closeError)
	}
	// If the WebRTC connection has already been closed (due to failure or disconnect),
	// this call will return an error that isn't typed. We don't check the error because
	// closing an already closed connection isn't an issue for us.
	_ = c.rtc.Close()

	// Waiting for pion/webrtc to report closed state on both of these
	// ensures no goroutine leaks.
	if c.rtc.ConnectionState() != webrtc.PeerConnectionStateNew {
		c.opts.Logger.Debug(context.Background(), "waiting for rtc connection close...")
		<-c.closedRTC
	}
	if c.rtc.ICEConnectionState() != webrtc.ICEConnectionStateNew {
		c.opts.Logger.Debug(context.Background(), "waiting for ice connection close...")
		<-c.closedICE
	}

	// Waits for all DataChannels to exit before officially labeling as closed.
	// All logging, goroutines, and async functionality is cleaned up after this.
	c.dcClosedWaitGroup.Wait()

	c.opts.Logger.Debug(context.Background(), "closed")
	close(c.closed)
	return err
}
