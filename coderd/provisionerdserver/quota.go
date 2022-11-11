package provisionerdserver

import (
	"context"

	"github.com/coder/coder/provisionerd/proto"
)

type QuotaCommitter interface {
	CommitQuota(ctx context.Context, request *proto.CommitQuotaRequest) (*proto.CommitQuotaResponse, error)
}
