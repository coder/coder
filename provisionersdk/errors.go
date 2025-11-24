package provisionersdk

import (
	"fmt"

	"github.com/coder/coder/v2/provisionersdk/proto"
)

func ParseErrorf(format string, args ...any) *proto.ParseComplete {
	return &proto.ParseComplete{Error: fmt.Sprintf(format, args...)}
}

func PlanErrorf(format string, args ...any) *proto.PlanComplete {
	return &proto.PlanComplete{Error: fmt.Sprintf(format, args...)}
}

func GraphErrorf(format string, args ...any) *proto.GraphComplete {
	return &proto.GraphComplete{Error: fmt.Sprintf(format, args...)}
}

func ApplyErrorf(format string, args ...any) *proto.ApplyComplete {
	return &proto.ApplyComplete{Error: fmt.Sprintf(format, args...)}
}
