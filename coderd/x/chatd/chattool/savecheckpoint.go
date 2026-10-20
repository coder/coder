package chattool

import (
	"context"
	"fmt"
	"strings"

	"charm.land/fantasy"
)

const (
	// SaveCheckpointToolName names the tool that records the checkpoint a
	// fresh context window starts from.
	SaveCheckpointToolName = "save_checkpoint"
	// MaxCheckpointBytes matches the memory body limit, so the checkpoint
	// fits in a rollover lead-in.
	MaxCheckpointBytes = MaxMemoryBodyBytes
)

type saveCheckpointArgs struct {
	Checkpoint string `json:"checkpoint" description:"The full checkpoint text. It replaces the previous checkpoint."`
}

// SaveCheckpoint returns a tool whose successful call input is the chat's
// checkpoint. The tool stores nothing itself: chatd reads the newest
// successful call from the transcript when it starts a new context window.
func SaveCheckpoint() fantasy.AgentTool {
	return fantasy.NewAgentTool(SaveCheckpointToolName,
		"Save a concise checkpoint for future context windows: the goal, decisions, progress, learnings, next steps, "+
			"and the window and item IDs of the user requests you are solving and of important actions. "+
			"Each save replaces the previous checkpoint. A new context window starts with only the latest checkpoint, not the transcript. "+
			fmt.Sprintf("At most %d bytes.", MaxCheckpointBytes),
		func(_ context.Context, args saveCheckpointArgs, _ fantasy.ToolCall) (fantasy.ToolResponse, error) {
			if strings.TrimSpace(args.Checkpoint) == "" {
				return fantasy.NewTextErrorResponse("checkpoint must not be empty"), nil
			}
			if len(args.Checkpoint) > MaxCheckpointBytes {
				return fantasy.NewTextErrorResponse(fmt.Sprintf("checkpoint must be at most %d bytes, got %d; shorten it and save again", MaxCheckpointBytes, len(args.Checkpoint))), nil
			}
			return toolResponse(map[string]any{"saved": true, "bytes": len(args.Checkpoint)}), nil
		})
}
