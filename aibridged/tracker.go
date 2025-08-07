package aibridged

import (
	"context"
	"encoding/json"
	"fmt"

	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/aibridged/proto"
)

type Metadata map[string]any

type Tracker interface {
	TrackTokensUsage(ctx context.Context, sessionID, msgID string, model Model, promptTokens, completionTokens int64, metadata Metadata) error
	TrackPromptUsage(ctx context.Context, sessionID, msgID string, model Model, prompt string, metadata Metadata) error
	TrackToolUsage(ctx context.Context, sessionID, msgID string, model Model, name string, args any, injected bool, metadata Metadata) error
}

var _ Tracker = &DRPCTracker{}

// DRPCTracker tracks usage by calling RPC endpoints on a given dRPC client.
type DRPCTracker struct {
	client proto.DRPCAIBridgeDaemonClient
}

func NewDRPCTracker(client proto.DRPCAIBridgeDaemonClient) *DRPCTracker {
	return &DRPCTracker{client}
}

func (d *DRPCTracker) TrackTokensUsage(ctx context.Context, sessionID, msgID string, model Model, promptTokens, completionTokens int64, metadata Metadata) error {
	_, err := d.client.TrackTokenUsage(ctx, &proto.TrackTokenUsageRequest{
		SessionId:    sessionID,
		MsgId:        msgID,
		InputTokens:  promptTokens,
		OutputTokens: completionTokens,
		Model:        fmt.Sprintf("%s.%s", model.Provider, model.ModelName), // TODO: make first-class type in proto.
		// Other:        metadata, // TODO: implement map<string, any> in proto.
	})
	return err
}

func (d *DRPCTracker) TrackPromptUsage(ctx context.Context, sessionID, msgID string, model Model, prompt string, metadata Metadata) error {
	_, err := d.client.TrackUserPrompt(ctx, &proto.TrackUserPromptRequest{
		SessionId: sessionID,
		MsgId:     msgID,
		Prompt:    prompt,
		Model:     fmt.Sprintf("%s.%s", model.Provider, model.ModelName), // TODO: make first-class type in proto.
	})
	return err
}

func (d *DRPCTracker) TrackToolUsage(ctx context.Context, sessionID, msgID string, model Model, name string, args any, injected bool, metadata Metadata) error {
	var (
		serialized []byte
		err        error
	)

	switch val := args.(type) {
	case string:
		serialized = []byte(val)
	case []byte:
		serialized = val
	default:
		serialized, err = json.Marshal(args)
		if err != nil {
			return xerrors.Errorf("marshal tool usage args: %w", err)
		}
	}

	_, err = d.client.TrackToolUsage(ctx, &proto.TrackToolUsageRequest{
		SessionId: sessionID,
		MsgId:     msgID,
		Tool:      name,
		Model:     fmt.Sprintf("%s.%s", model.Provider, model.ModelName), // TODO: make first-class type in proto.
		Input:     string(serialized),
		Injected:  injected,
	})
	return err
}
