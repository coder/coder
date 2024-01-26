package agentssh

import (
	"context"
	"strings"
	"sync"

	"github.com/gliderlabs/ssh"
	"go.uber.org/atomic"
	gossh "golang.org/x/crypto/ssh"

	"cdr.dev/slog"
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

	// If we do get a port, we should be able to get the matching PID and from
	// there look up the invocation.
	cmdline, err := getListeningPortProcessCmdline(d.DestPort)
	if err != nil {
		logger.Warn(ctx, "failed to inspect port",
			slog.F("destination_port", d.DestPort),
			slog.Error(err))
		return newChannel
	}

	// If this is not JetBrains, then we do not need to do anything special.  We
	// attempt to match on something that appears unique to JetBrains software.
	if !strings.Contains(strings.ToLower(cmdline), strings.ToLower(MagicProcessCmdlineJetBrains)) {
		return newChannel
	}

	logger.Debug(ctx, "discovered forwarded JetBrains process",
		slog.F("destination_port", d.DestPort))

	return &JetbrainsChannelWatcher{
		NewChannel:       newChannel,
		jetbrainsCounter: counter,
		logger:           logger.With(slog.F("destination_port", d.DestPort)),
	}
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
