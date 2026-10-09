import type { ChatACPTranscriptMessage } from "#/api/typesGenerated";

export const MockACPSpawnArgs = {
	agent: { harness: "claude-code", config: { model: "sonnet" } },
	working_directory: "/home/coder/project",
	prompt: "Find why the unit tests are failing.",
};

export const MockACPSession = {
	session_id: "8f2d6005-1b95-451e-8b56-a960d42c4805",
	harness: "claude-code",
	harness_display_name: "Claude Code",
	status: "running",
};

export const MockACPTranscript = [
	{ role: "user", content: [{ type: "text", text: MockACPSpawnArgs.prompt }] },
	{
		role: "assistant",
		content: [
			{
				type: "reasoning",
				text: "I will compare the response with the expected value.",
			},
			{ type: "text", text: "I found a mismatch in the expected response." },
			{
				type: "tool-call",
				tool_call_id: "run-tests",
				tool_name: "Run unit tests",
				args: { command: "go test ./agent/..." },
			},
		],
	},
	{
		role: "tool",
		content: [
			{
				type: "tool-result",
				tool_call_id: "run-tests",
				tool_name: "Run unit tests",
				result: {
					status: "completed",
					content: [
						{
							type: "content",
							content: {
								type: "text",
								text: "ok github.com/coder/coder/v2/agent",
							},
						},
					],
				},
			},
		],
	},
	{
		role: "assistant",
		content: [
			{
				type: "text",
				text: "Updated the response expectation. All affected tests pass.",
			},
		],
	},
] satisfies readonly ChatACPTranscriptMessage[];

export const MockACPWaitResult = {
	...MockACPSession,
	status: "idle",
	timed_out: false,
	history_complete: true,
	messages: MockACPTranscript,
};

export const MockACPWaitingResult = {
	...MockACPWaitResult,
	status: "running",
	messages: MockACPTranscript.slice(0, -1).map((message) => ({
		...message,
		content: message.content.map((part) =>
			part.type === "tool-result"
				? { ...part, result: { status: "in_progress" } }
				: part,
		),
	})),
};

const MockACPLoginErrorMessage =
	"Login expired. Run /login to sign in again, or re-authenticate your Anthropic profile.";

const MockACPLoginError = JSON.stringify({
	code: -32603,
	message: MockACPLoginErrorMessage,
	data: { errorKind: "invalid_request" },
});

export const MockACPExpiredLoginResult = {
	...MockACPWaitResult,
	status: "error",
	error: MockACPLoginError,
	messages: [
		{
			role: "assistant",
			content: [
				{
					type: "text",
					text: MockACPLoginErrorMessage,
				},
			],
		},
	] satisfies readonly ChatACPTranscriptMessage[],
};
