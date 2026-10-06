package tailnet

import (
	"slices"

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
	if slices.Contains(req.ReadyForHandshake, nil) {
		return xerrors.New("ready_for_handshake entries must not be nil")
	}
	return nil
}

func validateUpdateSelf(req *proto.CoordinateRequest) error {
	if upd := req.GetUpdateSelf(); upd != nil && upd.GetNode() == nil {
		return xerrors.New("update_self node is required")
	}
	return nil
}
