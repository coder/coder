package agentconnmock

import (
	"context"
	"reflect"

	gomock "go.uber.org/mock/gomock"

	"github.com/coder/coder/v2/codersdk/workspacesdk"
)

var _ workspacesdk.ProcessTokenProber = (*MockAgentConn)(nil)

// ProcessByToken mocks the optional ProcessTokenProber capability.
// Hand-written because mockgen only generates the AgentConn
// contract, which deliberately excludes the probe.
func (m *MockAgentConn) ProcessByToken(ctx context.Context, token string) (workspacesdk.ProcessByTokenResponse, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "ProcessByToken", ctx, token)
	ret0, _ := ret[0].(workspacesdk.ProcessByTokenResponse)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// ProcessByToken indicates an expected call of ProcessByToken.
func (mr *MockAgentConnMockRecorder) ProcessByToken(ctx, token any) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "ProcessByToken", reflect.TypeOf((*MockAgentConn)(nil).ProcessByToken), ctx, token)
}
