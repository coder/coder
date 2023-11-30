package agentssh

import (
	"cdr.dev/slog"
	"go.uber.org/atomic"
	gossh "golang.org/x/crypto/ssh"
)

type localForwardChannelData struct {
	DestAddr string
	DestPort uint32

	OriginAddr string
	OriginPort uint32
}

type ChannelAcceptWatcher struct {
	gossh.NewChannel
	jetbrainsCounter *atomic.Int64
}

func NewChannelAcceptWatcher(logger slog.Logger, newChannel gossh.NewChannel, counter *atomic.Int64) gossh.NewChannel {
	d := localForwardChannelData{}
	if err := gossh.Unmarshal(newChannel.ExtraData(), &d); err != nil {
		// If the data fails to unmarshal, do nothing
		return newChannel
	}

	//if !jetbrains {
	// If this isn't jetbrains, then we don't need to do anything special.
	//return newChannel
	//}

	return &ChannelAcceptWatcher{
		NewChannel:       newChannel,
		jetbrainsCounter: counter,
	}
}

func (w *ChannelAcceptWatcher) Accept() (gossh.Channel, <-chan *gossh.Request, error) {
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
	done func()
}

func (c *ChannelOnClose) Close() error {
	c.done()
	return c.Channel.Close()
}
