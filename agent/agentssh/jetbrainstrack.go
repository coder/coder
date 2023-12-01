package agentssh

import (
	"strings"
	"sync"

	"cdr.dev/slog"
	"github.com/gliderlabs/ssh"
	"go.uber.org/atomic"
	gossh "golang.org/x/crypto/ssh"
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
}

func NewJetbrainsChannelWatcher(ctx ssh.Context, logger slog.Logger, newChannel gossh.NewChannel, counter *atomic.Int64) gossh.NewChannel {
	d := localForwardChannelData{}
	if err := gossh.Unmarshal(newChannel.ExtraData(), &d); err != nil {
		// If the data fails to unmarshal, do nothing.
		return newChannel
	}

	// If we do get a port, we should be able to get the matching PID and from
	// there look up the invocation.
	cmdline, err := getListeningPortProcessCmdline(d.DestPort)
	if err != nil {
		logger.Warn(ctx, "port inspection failed",
			slog.F("destination_port", d.DestPort),
			slog.Error(err))
		return newChannel
	}
	logger.Debug(ctx, "checking forwarded process",
		slog.F("cmdline", cmdline),
		slog.F("destination_port", d.DestPort))

	// If this is not JetBrains, then we do not need to do anything special.  We
	// attempt to match on something that appears unique to JetBrains software and
	// the vendor name flag seems like it might be a reasonable choice.
	if !strings.Contains(strings.ToLower(cmdline), "idea.vendor.name=jetbrains") {
		return newChannel
	}

	logger.Debug(ctx, "discovered forwarded JetBrains process",
		slog.F("destination_port", d.DestPort))

	return &JetbrainsChannelWatcher{
		NewChannel:       newChannel,
		jetbrainsCounter: counter,
	}
}

func (w *JetbrainsChannelWatcher) Accept() (gossh.Channel, <-chan *gossh.Request, error) {
	c, r, err := w.NewChannel.Accept()
	if err != nil {
		return c, r, err
	}
	w.jetbrainsCounter.Add(1)

	return &ChannelOnClose{
		Channel: c,
		done: func() {
			w.jetbrainsCounter.Add(-1)
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
