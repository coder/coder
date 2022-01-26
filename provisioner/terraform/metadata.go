package terraform

import (
	"context"

	"github.com/coder/coder/provisionersdk/proto"
)

func (*terraform) Metadata(context.Context, *proto.Metadata_Request) (*proto.Metadata_Response, error) {
	return &proto.Metadata_Response{
		Id: "terraform",
	}, nil
}
