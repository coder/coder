package agentssh

import (
	"context"
	"strings"
	"sync"

	"github.com/gliderlabs/ssh"
	"go.uber.org/atomic"
	gossh "golang.org/x/crypto/ssh"

	"cdr.dev/slog"
	"github.com/coder/coder/v2/coderd/util/slice"
)

// localForwardChannelData is copied from the ssh package.
type localForwardChannelData struct {
	DestAddr string
	DestPort uint32

	OriginAddr string
	OriginPort uint32
}

// JetbrainsChannelWatcher is used to track JetBrains port forwarded (Gateway)
// channels. If the port forward is something other than JetBrains, this struct
// is a noop.
type JetbrainsChannelWatcher struct {
	gossh.NewChannel
	jetbrainsCounter *atomic.Int64
	logger           slog.Logger
}

func NewJetbrainsChannelWatcher(ctx ssh.Context, logger slog.Logger, newChannel gossh.NewChannel, counter *atomic.Int64) gossh.NewChannel {
	d := localForwardChannelData{}
	if err := gossh.Unmarshal(newChannel.ExtraData(), &d); err != nil {
		// If the data fails to unmarshal, do nothing.
		logger.Warn(ctx, "failed to unmarshal port forward data", slog.Error(err))
		return newChannel
	}

	// If we do get a port, we should be able to get the matching PID(s) and from
	// there look up the invocation(s).  For now, ignore the address because we
	// would need to resolve the address (for example `localhost` has a number of
	// possibilities, or ::1 might route to ::, and so on).  The consequence is
	// that if a user has another forwarded process listening on a different
	// address but the same port as an active JetBrains process then the count
	// will be inflated by however many processes are doing that.
	cmdlines, err := getListeningPortProcessCmdlines(d.DestPort)

	// If any of these are JetBrains processes, wrap the channel in a watcher so
	// we can increment and decrement the session count when the channel
	// opens/closes.  We attempt to match on something that appears unique to
	// JetBrains software.  As mentioned above, this can give false positives
	// since we are not checking the address of each process.
	if slice.ContainsCompare(cmdlines, strings.ToLower(MagicProcessCmdlineJetBrains),
		func(magic, cmdline string) bool {
			return strings.Contains(strings.ToLower(cmdline), magic)
		}) {
		logger.Debug(ctx, "discovered forwarded JetBrains process",
			slog.F("destination_address", d.DestAddr),
			slog.F("destination_port", d.DestPort))

		return &JetbrainsChannelWatcher{
			NewChannel:       newChannel,
			jetbrainsCounter: counter,
			logger: logger.With(
				slog.F("destination_address", d.DestAddr),
				slog.F("destination_port", d.DestPort),
			),
		}
	}

	// We do not want to break the forwarding if we were unable to inspect the
	// port, so only log any errors.  The consequence of failing port inspection
	// is that the JetBrains session count might be lower than it should be.
	if err != nil {
		logger.Warn(ctx, "failed to inspect port",
			slog.F("destination_addr", d.DestAddr),
			slog.F("destination_port", d.DestPort),
			slog.Error(err))
	}

	// Either not a JetBrains process or we failed to figure it out.  Do nothing;
	// just return the channel as-is.
	return newChannel
}

func (w *JetbrainsChannelWatcher) Accept() (gossh.Channel, <-chan *gossh.Request, error) {
	c, r, err := w.NewChannel.Accept()
	if err != nil {
		return c, r, err
	}
	w.jetbrainsCounter.Add(1)
	// nolint: gocritic // JetBrains is a proper noun and should be capitalized
	w.logger.Debug(context.Background(), "JetBrains watcher accepted channel")

	return &ChannelOnClose{
		Channel: c,
		done: func() {
			w.jetbrainsCounter.Add(-1)
			// nolint: gocritic // JetBrains is a proper noun and should be capitalized
			w.logger.Debug(context.Background(), "JetBrains watcher channel closed")
		},
	}, r, err
}

type ChannelOnClose struct {
	gossh.Channel
	// once ensures close only decrements the counter once.
	// Because close can be called multiple times.
	once sync.Once
	done func()
}

func (c *ChannelOnClose) Close() error {
	c.once.Do(c.done)
	return c.Channel.Close()
}
