package agentssh

import (
	"context"
	"strings"
	"sync"

	"github.com/gliderlabs/ssh"
	"github.com/google/uuid"
	gossh "golang.org/x/crypto/ssh"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/codersdk"
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
//
// Sessions are counted here, not in sessionHandler: JetBrains opens hundreds
// of ssh sessions but only one persistent forwarded channel.
type JetbrainsChannelWatcher struct {
	gossh.NewChannel
	startSession     func(MagicSessionType) (endSession func())
	logger           slog.Logger
	originAddr       string
	reportConnection reportConnectionFunc
}

func NewJetbrainsChannelWatcher(ctx ssh.Context, logger slog.Logger, reportConnection reportConnectionFunc, newChannel gossh.NewChannel, startSession func(MagicSessionType) (endSession func())) gossh.NewChannel {
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
	if !isJetbrainsProcess(cmdline) {
		return newChannel
	}

	logger.Debug(ctx, "discovered forwarded JetBrains process",
		slog.F("destination_port", d.DestPort))

	return &JetbrainsChannelWatcher{
		NewChannel:       newChannel,
		startSession:     startSession,
		logger:           logger.With(slog.F("destination_port", d.DestPort)),
		originAddr:       d.OriginAddr,
		reportConnection: reportConnection,
	}
}

func (w *JetbrainsChannelWatcher) Accept() (gossh.Channel, <-chan *gossh.Request, error) {
	disconnected := w.reportConnection(uuid.New(), MagicSessionTypeJetBrains, w.originAddr)

	c, r, err := w.NewChannel.Accept()
	if err != nil {
		disconnected(1, err.Error())
		return c, r, err
	}
	endSession := w.startSession(MagicSessionTypeJetBrains)
	// nolint: gocritic // JetBrains is a proper noun and should be capitalized
	w.logger.Debug(context.Background(), "JetBrains watcher accepted channel")

	return &ChannelOnClose{
		Channel: c,
		done: func() {
			endSession()
			disconnected(0, "normal close")
			// nolint: gocritic // JetBrains is a proper noun and should be capitalized
			w.logger.Debug(context.Background(), "JetBrains channel closed",
				codersdk.ConnectionDirectionAgentToClient.SlogField(),
				codersdk.DisconnectReasonGraceful.SlogField(),
				codersdk.DisconnectReasonGraceful.SlogExpectedField(),
			)
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

func isJetbrainsProcess(cmdline string) bool {
	opts := []string{
		MagicProcessCmdlineJetBrains,
		MagicProcessCmdlineToolbox,
		MagicProcessCmdlineGateway,
	}

	for _, opt := range opts {
		if strings.Contains(strings.ToLower(cmdline), strings.ToLower(opt)) {
			return true
		}
	}
	return false
}
