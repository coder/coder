package agentacp

import (
	"context"

	acp "github.com/coder/acp-go-sdk"
)

// client implements the callbacks shared by discovery and session clients.
type client struct{}

// discoveryClient ignores session updates because probes retain only metadata.
type discoveryClient struct{ client }

func (*discoveryClient) SessionUpdate(context.Context, acp.SessionNotification) error { return nil }

func (*client) RequestPermission(ctx context.Context, req acp.RequestPermissionRequest) (acp.RequestPermissionResponse, error) {
	if ctx.Err() == nil {
		for _, kind := range []acp.PermissionOptionKind{acp.PermissionOptionKindAllowAlways, acp.PermissionOptionKindAllowOnce} {
			for _, option := range req.Options {
				if option.Kind == kind {
					return acp.RequestPermissionResponse{Outcome: acp.RequestPermissionOutcome{Selected: &acp.RequestPermissionOutcomeSelected{OptionId: option.OptionId}}}, nil
				}
			}
		}
	}
	//nolint:misspell // ACP names this protocol variant Cancelled.
	return acp.RequestPermissionResponse{Outcome: acp.RequestPermissionOutcome{Cancelled: &acp.RequestPermissionOutcomeCancelled{}}}, nil
}

func (*client) ReadTextFile(context.Context, acp.ReadTextFileRequest) (acp.ReadTextFileResponse, error) {
	return acp.ReadTextFileResponse{}, acp.NewMethodNotFound("fs/read_text_file")
}

func (*client) WriteTextFile(context.Context, acp.WriteTextFileRequest) (acp.WriteTextFileResponse, error) {
	return acp.WriteTextFileResponse{}, acp.NewMethodNotFound("fs/write_text_file")
}

func (*client) CreateTerminal(context.Context, acp.CreateTerminalRequest) (acp.CreateTerminalResponse, error) {
	return acp.CreateTerminalResponse{}, acp.NewMethodNotFound("terminal/create")
}

func (*client) KillTerminal(context.Context, acp.KillTerminalRequest) (acp.KillTerminalResponse, error) {
	return acp.KillTerminalResponse{}, acp.NewMethodNotFound("terminal/kill")
}

func (*client) TerminalOutput(context.Context, acp.TerminalOutputRequest) (acp.TerminalOutputResponse, error) {
	return acp.TerminalOutputResponse{}, acp.NewMethodNotFound("terminal/output")
}

func (*client) ReleaseTerminal(context.Context, acp.ReleaseTerminalRequest) (acp.ReleaseTerminalResponse, error) {
	return acp.ReleaseTerminalResponse{}, acp.NewMethodNotFound("terminal/release")
}

func (*client) WaitForTerminalExit(context.Context, acp.WaitForTerminalExitRequest) (acp.WaitForTerminalExitResponse, error) {
	return acp.WaitForTerminalExitResponse{}, acp.NewMethodNotFound("terminal/wait_for_exit")
}

// Verify at compile time that *discoveryClient implements acp.Client.
var _ acp.Client = (*discoveryClient)(nil)
