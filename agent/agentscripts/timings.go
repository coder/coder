package agentscripts

import (
	"time"

	"github.com/coder/coder/v2/agent/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type timingSpan struct {
	displayName string
	start, end  time.Time
	exitCode    int32
}

func (ts *timingSpan) ToProto() *proto.Timing {
	return &proto.Timing{
		DisplayName: ts.displayName,
		Start:       timestamppb.New(ts.start),
		End:         timestamppb.New(ts.end),
		ExitCode:    ts.exitCode,
	}
}
