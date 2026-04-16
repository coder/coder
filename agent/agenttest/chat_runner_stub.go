package agenttest

import (
	"context"

	agentproto "github.com/coder/coder/v2/agent/proto"
)

var drpcAgentUnimplementedServer = &agentproto.DRPCAgentUnimplementedServer{}

func (*FakeAgentAPI) ReportChatRunnerStatus(ctx context.Context, req *agentproto.ReportChatRunnerStatusRequest) (*agentproto.ReportChatRunnerStatusResponse, error) {
	return drpcAgentUnimplementedServer.ReportChatRunnerStatus(ctx, req)
}

func (*FakeAgentAPI) PollChatWork(ctx context.Context, req *agentproto.PollChatWorkRequest) (*agentproto.PollChatWorkResponse, error) {
	return drpcAgentUnimplementedServer.PollChatWork(ctx, req)
}

func (*FakeAgentAPI) AcquireChatLease(ctx context.Context, req *agentproto.AcquireChatLeaseRequest) (*agentproto.AcquireChatLeaseResponse, error) {
	return drpcAgentUnimplementedServer.AcquireChatLease(ctx, req)
}

func (*FakeAgentAPI) RenewChatLease(ctx context.Context, req *agentproto.RenewChatLeaseRequest) (*agentproto.RenewChatLeaseResponse, error) {
	return drpcAgentUnimplementedServer.RenewChatLease(ctx, req)
}

func (*FakeAgentAPI) ReleaseChatLease(ctx context.Context, req *agentproto.ReleaseChatLeaseRequest) (*agentproto.ReleaseChatLeaseResponse, error) {
	return drpcAgentUnimplementedServer.ReleaseChatLease(ctx, req)
}
