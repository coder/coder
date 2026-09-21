import type { ACPSession } from "#/api/typesGenerated";

export const MockACPSession: ACPSession = {
	session_id: "8f2d6005-1b95-451e-8b56-a960d42c4805",
	workspace_agent_id: "e7e2c400-8475-49bf-b8f1-8d7d3a5a86c3",
	parent_chat_id: "9ed7b65a-cc8a-49c2-af3d-8ae5fb2d0a40",
	agent: "claude_code",
	title: "Investigate failing tests",
	status: "running",
	created_at: "2026-09-21T12:00:00Z",
	updated_at: "2026-09-21T12:00:01Z",
	version: 3,
	queued: 0,
	entries: [
		{
			id: "prompt",
			role: "user",
			kind: "text",
			text: "Find why the unit tests are failing.",
		},
		{
			id: "response",
			role: "assistant",
			kind: "text",
			text: "I found a mismatch in the expected response. I am running the affected tests.",
		},
		{
			id: "tool",
			role: "assistant",
			kind: "tool",
			text: "",
			title: "Run unit tests",
			status: "in_progress",
			input: { command: "go test ./agent/..." },
		},
	],
};
