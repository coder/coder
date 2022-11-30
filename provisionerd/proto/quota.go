package proto

import context "context"

type QuotaCommitter interface {
	CommitQuota(ctx context.Context, request *CommitQuotaRequest) (*CommitQuotaResponse, error)
}
