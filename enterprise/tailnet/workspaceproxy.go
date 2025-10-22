package tailnet

import (
	"context"
	"net"

	"github.com/google/uuid"

	"cdr.dev/slog"
	"github.com/coder/coder/v2/apiversion"
	agpl "github.com/coder/coder/v2/tailnet"
)

type ClientService struct {
	*agpl.ClientService
}

// NewClientService returns a ClientService based on the given Coordinator pointer.  The pointer is
// loaded on each processed connection.
func NewClientService(options agpl.ClientServiceOptions) (*ClientService, error) {
	s, err := agpl.NewClientService(options)
	if err != nil {
		return nil, err
	}
	return &ClientService{ClientService: s}, nil
}

func (s *ClientService) ServeMultiAgentClient(ctx context.Context, version string, conn net.Conn, id uuid.UUID) error {
	major, _, err := apiversion.Parse(version)
	if err != nil {
		s.Logger.Warn(ctx, "serve client called with unparsable version", slog.Error(err))
		return err
	}
	switch major {
	case 2:
		auth := agpl.SingleTailnetCoordinateeAuth{}
		streamID := agpl.StreamID{
			Name: id.String(),
			ID:   id,
			Auth: auth,
		}
		return s.ServeConnV2(ctx, conn, streamID)
	default:
		s.Logger.Warn(ctx, "serve client called with unsupported version", slog.F("version", version))
		return agpl.ErrUnsupportedVersion
	}
}
