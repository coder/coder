package tailnet

import (
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/tailnet/proto"
)

// ValidateCoordinateRequest checks the structural invariants required by the
// coordinator before authorization or state updates.
func ValidateCoordinateRequest(req *proto.CoordinateRequest) error {
	if req == nil {
		return xerrors.New("coordinate request is required")
	}
	if err := validateUpdateSelf(req); err != nil {
		return err
	}
	for _, rfh := range req.ReadyForHandshake {
		if rfh == nil {
			return xerrors.New("ready_for_handshake entry is required")
		}
	}
	return nil
}

func validateUpdateSelf(req *proto.CoordinateRequest) error {
	if req != nil && req.UpdateSelf != nil && req.UpdateSelf.Node == nil {
		return xerrors.New("update_self node is required")
	}
	return nil
}
