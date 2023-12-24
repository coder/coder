package agentapi

import (
	"context"
	"time"

	"golang.org/x/xerrors"
	"tailscale.com/tailcfg"

	agentproto "github.com/coder/coder/v2/agent/proto"
	"github.com/coder/coder/v2/tailnet"
	tailnetproto "github.com/coder/coder/v2/tailnet/proto"
)

type TailnetAPI struct {
	Ctx                    context.Context
	DerpMapFn              func() *tailcfg.DERPMap
	DerpMapUpdateFrequency time.Duration
}

func (a *TailnetAPI) StreamDERPMaps(_ *tailnetproto.StreamDERPMapsRequest, stream agentproto.DRPCAgent_StreamDERPMapsStream) error {
	defer stream.Close()

	ticker := time.NewTicker(a.DerpMapUpdateFrequency)
	defer ticker.Stop()

	var lastDERPMap *tailcfg.DERPMap
	for {
		derpMap := a.DerpMapFn()
		if lastDERPMap == nil || !tailnet.CompareDERPMaps(lastDERPMap, derpMap) {
			protoDERPMap := tailnet.DERPMapToProto(derpMap)
			err := stream.Send(protoDERPMap)
			if err != nil {
				return xerrors.Errorf("send derp map: %w", err)
			}
			lastDERPMap = derpMap
		}

		ticker.Reset(a.DerpMapUpdateFrequency)
		select {
		case <-stream.Context().Done():
			return nil
		case <-a.Ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}

func (*TailnetAPI) CoordinateTailnet(_ agentproto.DRPCAgent_CoordinateTailnetStream) error {
	// TODO: implement this
	return xerrors.New("CoordinateTailnet is unimplemented")
}
