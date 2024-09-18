package agentscripts

import (
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/coder/coder/v2/agent/proto"
)

type TimingSpan struct {
	displayName string
	start, end  time.Time
	exitCode    int32
}

func (ts *TimingSpan) ToProto() *proto.Timing {
	return &proto.Timing{
		DisplayName: ts.displayName,
		Start:       timestamppb.New(ts.start),
		End:         timestamppb.New(ts.end),
		ExitCode:    ts.exitCode,
	}
}
