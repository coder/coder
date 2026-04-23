import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, spyOn, userEvent, waitFor, within } from "storybook/test";
import { API } from "#/api/api";
import type * as TypesGen from "#/api/typesGenerated";
import { DebugPanel } from "./DebugPanel";

const FIXTURE_NOW = Date.parse("2026-03-05T12:00:10.000Z");

const CHAT_ID = "debug-chat-1";

const makeRunSummary = (
	overrides: Partial<TypesGen.ChatDebugRunSummary>,
): TypesGen.ChatDebugRunSummary => ({
	id: "run-1",
	chat_id: CHAT_ID,
	kind: "chat_turn",
	status: "completed",
	provider: "openai",
	model: "gpt-4",
	summary: {},
	started_at: "2026-03-05T12:00:05Z",
	updated_at: "2026-03-05T12:00:08Z",
	finished_at: "2026-03-05T12:00:08Z",
	...overrides,
});

const makeStep = (
	overrides: Partial<TypesGen.ChatDebugStep>,
): TypesGen.ChatDebugStep => ({
	id: "step-1",
	run_id: "run-1",
	chat_id: CHAT_ID,
	step_number: 1,
	operation: "stream",
	status: "completed",
	normalized_request: { model: "gpt-4", prompt: "Hello" },
	normalized_response: { content: "Hi there!", finish_reason: "stop" },
	usage: { prompt_tokens: "10", completion_tokens: "5", total_tokens: "15" },
	attempts: [],
	metadata: { provider: "openai" },
	started_at: "2026-03-05T12:00:06Z",
	updated_at: "2026-03-05T12:00:08Z",
	finished_at: "2026-03-05T12:00:08Z",
	...overrides,
});

const makeRun = (
	overrides: Partial<TypesGen.ChatDebugRun>,
): TypesGen.ChatDebugRun => ({
	id: "run-1",
	chat_id: CHAT_ID,
	kind: "chat_turn",
	status: "completed",
	provider: "openai",
	model: "gpt-4",
	summary: { result: "Generated response successfully" },
	started_at: "2026-03-05T12:00:05Z",
	updated_at: "2026-03-05T12:00:08Z",
	finished_at: "2026-03-05T12:00:08Z",
	steps: [makeStep({})],
	...overrides,
});

type StoryAttempt = Record<string, unknown>;

const makeAttempts = (
	attempts: readonly Record<string, unknown>[],
): TypesGen.ChatDebugStep["attempts"] => {
	return attempts.map((attempt, index) => ({
		...attempt,
		attempt_number:
			typeof attempt.attempt_number === "number"
				? attempt.attempt_number
				: typeof attempt.number === "number"
					? attempt.number
					: index + 1,
		status: typeof attempt.status === "string" ? attempt.status : "completed",
		started_at:
			typeof attempt.started_at === "string"
				? attempt.started_at
				: "2026-03-05T12:00:06Z",
	})) as readonly StoryAttempt[];
};

const makeLargeRecord = (
	prefix: string,
	count: number,
): Record<string, string> => {
	return Object.fromEntries(
		Array.from({ length: count }, (_, index) => [
			`${prefix}_${index + 1}`,
			`${prefix}-value-${index + 1}-${"x".repeat(24)}`,
		]),
	);
};

type StoryCanvas = ReturnType<typeof within>;
type StoryUser = ReturnType<typeof userEvent.setup>;

// Story fixtures use structured normalized payloads even though the generated
// API type still models them as string records.
const makeNormalizedPayloadFixture = (
	payload: Record<string, unknown>,
): TypesGen.ChatDebugStep["normalized_request"] => {
	return payload as TypesGen.ChatDebugStep["normalized_request"];
};

// DebugRunCard renders DebugStepCard with defaultOpen={false}, so nested step
// content is only visible after the step trigger is opened explicitly.
const expandStep = async (
	canvas: StoryCanvas,
	user: StoryUser,
	stepName: RegExp | string = /Step 1/i,
) => {
	const stepTrigger = await canvas.findByRole("button", { name: stepName });
	await user.click(stepTrigger);
	return stepTrigger;
};

// ---------------------------------------------------------------------------
// Rich-payload fixtures (messages, tools, usage, firstMessage).
// ---------------------------------------------------------------------------

const richRequest: Record<string, string> = {
	model: "gpt-4",
	messages: JSON.stringify([
		{ role: "system", content: "You are a helpful coding assistant." },
		{
			role: "user",
			content: "Write me a hello world function in Python",
		},
	]),
	tools: JSON.stringify([
		{
			type: "function",
			function: {
				name: "run_code",
				description: "Execute Python code in a sandbox",
			},
		},
		{
			type: "function",
			function: {
				name: "search_docs",
				description: "Search documentation",
			},
		},
	]),
	temperature: "0.7",
	max_output_tokens: "4096",
	tool_choice: "auto",
};

const richResponse: Record<string, string> = {
	content:
		"Here's a hello world function:\n\n```python\ndef hello():\n    print('Hello, world!')\n```",
	finish_reason: "stop",
	model: "gpt-4",
};

const toolCallResponse: Record<string, string> = {
	content: "",
	tool_calls: JSON.stringify([
		{
			id: "call_1",
			function: {
				name: "run_code",
				arguments: '{"code":"print(\'hello\')"}',
			},
		},
	]),
	finish_reason: "tool_calls",
	model: "gpt-4",
};

// ---------------------------------------------------------------------------
// Pre-built run details.
// ---------------------------------------------------------------------------

const successfulRunDetail = makeRun({
	summary: {
		result: "Generated response successfully",
		latency: "5s",
	},
	steps: [
		makeStep({
			attempts: makeAttempts([
				{
					attempt_number: 1,
					status: "completed",
					raw_request: {
						url: "https://api.openai.com/v1/chat/completions",
						method: "POST",
					},
					raw_response: {
						status: "200",
						request_id: "req-success-1",
					},
					duration_ms: 1500,
					started_at: "2026-03-05T12:00:06Z",
					finished_at: "2026-03-05T12:00:08Z",
				},
			]),
			metadata: {
				provider: "openai",
				region: "us-east-1",
			},
		}),
	],
});

const richRunDetail = makeRun({
	id: "run-rich",
	summary: {
		first_message: "Write me a hello world function in Python",
		prompt_tokens: "150",
		completion_tokens: "42",
	},
	steps: [
		makeStep({
			id: "step-rich-1",
			run_id: "run-rich",
			normalized_request: richRequest,
			normalized_response: richResponse,
			usage: {
				prompt_tokens: "150",
				completion_tokens: "42",
				total_tokens: "192",
			},
			attempts: makeAttempts([
				{
					attempt_number: 1,
					status: "completed",
					raw_request: {
						url: "https://api.openai.com/v1/chat/completions",
					},
					raw_response: { status: "200" },
					duration_ms: 2200,
					started_at: "2026-03-05T12:00:06Z",
					finished_at: "2026-03-05T12:00:08Z",
				},
			]),
		}),
	],
});

const toolCallRunDetail = makeRun({
	id: "run-tool",
	summary: {
		first_message: "Run some code for me",
	},
	steps: [
		makeStep({
			id: "step-tool-1",
			run_id: "run-tool",
			normalized_request: richRequest,
			normalized_response: toolCallResponse,
			usage: {
				prompt_tokens: "200",
				completion_tokens: "30",
				total_tokens: "230",
			},
			attempts: makeAttempts([]),
		}),
	],
});

const multiStepRunDetail = makeRun({
	id: "run-2",
	status: "completed",
	started_at: "2026-03-02T09:00:00Z",
	updated_at: "2026-03-02T09:00:12Z",
	finished_at: "2026-03-02T09:00:12Z",
	summary: {
		result: "Recovered after retries",
		retries: "2",
	},
	steps: [
		makeStep({
			id: "step-2-1",
			run_id: "run-2",
			step_number: 1,
			status: "completed",
			normalized_request: {
				model: "gpt-4",
				prompt: "Retry this call until success",
			},
			normalized_response: {
				content: "Retry succeeded on attempt 3",
				finish_reason: "stop",
			},
			attempts: makeAttempts([
				{
					attempt_number: 1,
					status: "failed",
					raw_request: { url: "https://api.openai.com/v1/chat/completions" },
					raw_response: { status: "500" },
					error: { message: "upstream timeout" },
					duration_ms: 1200,
					started_at: "2026-03-02T09:00:01Z",
					finished_at: "2026-03-02T09:00:02.200Z",
				},
				{
					attempt_number: 2,
					status: "failed",
					raw_request: { url: "https://api.openai.com/v1/chat/completions" },
					raw_response: { status: "429" },
					error: { message: "rate limited" },
					duration_ms: 900,
					started_at: "2026-03-02T09:00:03Z",
					finished_at: "2026-03-02T09:00:03.900Z",
				},
				{
					attempt_number: 3,
					status: "succeeded",
					raw_request: { url: "https://api.openai.com/v1/chat/completions" },
					raw_response: { status: "200" },
					duration_ms: 1400,
					started_at: "2026-03-02T09:00:04Z",
					finished_at: "2026-03-02T09:00:05.400Z",
				},
			]),
		}),
		makeStep({
			id: "step-2-2",
			run_id: "run-2",
			step_number: 2,
			operation: "generate",
			status: "completed",
			normalized_request: { action: "annotate", content: "Final answer" },
			normalized_response: { result: "Annotated response" },
			attempts: makeAttempts([
				{
					attempt_number: 1,
					status: "completed",
					raw_request: { phase: "generate" },
					raw_response: { status: "200" },
					duration_ms: 500,
					started_at: "2026-03-02T09:00:06Z",
					finished_at: "2026-03-02T09:00:06.500Z",
				},
			]),
		}),
	],
});

const errorRunDetail = makeRun({
	id: "run-3",
	status: "error",
	started_at: "2026-03-03T14:00:00Z",
	updated_at: "2026-03-03T14:00:07Z",
	finished_at: "2026-03-03T14:00:07Z",
	summary: {
		result: "Provider request failed",
		authorization: "[REDACTED]",
	},
	steps: [
		makeStep({
			id: "step-3-1",
			run_id: "run-3",
			status: "error",
			normalized_request: {
				model: "gpt-4",
				authorization: "[REDACTED]",
				x_trace: "trace-123",
			},
			normalized_response: { status: "401", detail: "Unauthorized" },
			error: {
				message: "Provider request failed",
				code: "upstream_unauthorized",
			},
			attempts: makeAttempts([
				{
					attempt_number: 1,
					status: "failed",
					raw_request: {
						authorization: "***",
						url: "https://api.openai.com/v1/chat/completions",
					},
					raw_response: { status: "401" },
					error: { message: "invalid auth header" },
					duration_ms: 800,
					started_at: "2026-03-03T14:00:01Z",
					finished_at: "2026-03-03T14:00:01.800Z",
				},
			]),
		}),
	],
});

const longPayloadRunDetail = makeRun({
	id: "run-4",
	status: "completed",
	started_at: "2026-03-04T08:30:00Z",
	updated_at: "2026-03-04T08:30:20Z",
	finished_at: "2026-03-04T08:30:20Z",
	summary: {
		result: "Large payload rendered",
		size: "large",
	},
	steps: [
		makeStep({
			id: "step-4-1",
			run_id: "run-4",
			normalized_request: makeLargeRecord("request", 24),
			normalized_response: makeLargeRecord("response", 24),
			metadata: makeLargeRecord("metadata", 12),
			usage: {
				prompt_tokens: "512",
				completion_tokens: "256",
				total_tokens: "768",
			},
			attempts: makeAttempts([
				{
					attempt_number: 1,
					status: "completed",
					raw_request: makeLargeRecord("raw_request_chunk", 20),
					raw_response: makeLargeRecord("raw_response_chunk", 20),
					duration_ms: 3200,
					started_at: "2026-03-04T08:30:02Z",
					finished_at: "2026-03-04T08:30:05.200Z",
				},
			]),
		}),
	],
});

const getAllRunDetails = () => [
	successfulRunDetail,
	richRunDetail,
	toolCallRunDetail,
	multiStepRunDetail,
	errorRunDetail,
	longPayloadRunDetail,
	backendShapeRunDetail,
];

const getAllRunSummaries = () =>
	getAllRunDetails().map((run) =>
		makeRunSummary({
			id: run.id,
			kind: run.kind,
			status: run.status,
			provider: run.provider,
			model: run.model,
			summary: run.summary,
			started_at: run.started_at,
			updated_at: run.updated_at,
			finished_at: run.finished_at,
		}),
	);

const getDebugRunDetailById = () =>
	new Map(getAllRunDetails().map((run) => [run.id, run]));

const debugRunsQueryKey = ["chats", CHAT_ID, "debug-runs"] as const;

const getSeededRunSummaries = (
	queries: readonly { key: readonly unknown[]; data: unknown }[] | undefined,
): TypesGen.ChatDebugRunSummary[] => {
	const seeded = queries?.find(
		(query) =>
			query.key.length === debugRunsQueryKey.length &&
			query.key.every((part, index) => part === debugRunsQueryKey[index]),
	)?.data;
	return Array.isArray(seeded)
		? [...(seeded as TypesGen.ChatDebugRunSummary[])]
		: getAllRunSummaries();
};

const meta: Meta<typeof DebugPanel> = {
	title: "pages/AgentsPage/DebugPanel",
	component: DebugPanel,
	args: {
		chatId: CHAT_ID,
		isVisible: true,
	},
	beforeEach: (ctx) => {
		const real = Date.now;
		Date.now = () => FIXTURE_NOW;
		const getChatDebugRunsMock = spyOn(
			API.experimental,
			"getChatDebugRuns",
		).mockResolvedValue(getSeededRunSummaries(ctx.parameters.queries));
		const getChatDebugRunMock = spyOn(
			API.experimental,
			"getChatDebugRun",
		).mockImplementation(async (_chatID, runID) => {
			return (
				getDebugRunDetailById().get(runID) ??
				makeRun({
					id: runID,
					summary: { result: `Unknown debug run fixture: ${runID}` },
					steps: [],
				})
			);
		});
		return () => {
			Date.now = real;
			getChatDebugRunsMock.mockRestore();
			getChatDebugRunMock.mockRestore();
		};
	},
	decorators: [
		(Story) => (
			<div style={{ height: 900, width: 560, padding: 16 }}>
				<Story />
			</div>
		),
	],
};

export default meta;
type Story = StoryObj<typeof DebugPanel>;

export const Empty: Story = {
	parameters: {
		queries: [
			{
				key: ["chats", CHAT_ID, "debug-runs"],
				data: [],
			},
		],
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(canvas.getByText(/no debug runs/i)).toBeInTheDocument();
	},
};

export const Disabled: Story = {
	args: {
		isVisible: false,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(
			canvas.getByText(/no debug runs recorded yet/i),
		).toBeInTheDocument();
	},
};

export const ErrorState: Story = {
	beforeEach: () => {
		const getChatDebugRunsMock = spyOn(
			API.experimental,
			"getChatDebugRuns",
		).mockRejectedValue(new Error("Network failure"));
		return () => {
			getChatDebugRunsMock.mockRestore();
		};
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		// `getErrorMessage` treats any object with a string `message`
		// property as an `ApiErrorResponse`, which includes plain `Error`
		// instances, so the rejection surfaces via `error.message`.
		await waitFor(() => {
			expect(canvas.getByText(/network failure/i)).toBeInTheDocument();
		});
	},
};

export const Loading: Story = {
	beforeEach: () => {
		const pendingRequest = () => new Promise<never>(() => {});
		const getChatDebugRunsMock = spyOn(
			API.experimental,
			"getChatDebugRuns",
		).mockImplementation(pendingRequest);
		return () => {
			getChatDebugRunsMock.mockRestore();
		};
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(canvas.getByText(/loading debug/i)).toBeInTheDocument();
	},
};

// ---------------------------------------------------------------------------
// Run-detail branch stories.
//
// After a run card expands, `DebugRunCard` renders one of three branches
// based on its detail query: a loading spinner, an error Alert, or the
// empty-steps fallback. Each story below pins the detail query into one
// of those states to lock in coverage of the branching logic.
// ---------------------------------------------------------------------------

const detailProbeRunId = "run-detail-probe";
const detailProbeSummary = makeRunSummary({
	id: detailProbeRunId,
	summary: { first_message: "Detail state probe" },
});

export const RunDetailLoading: Story = {
	parameters: {
		queries: [
			{
				key: ["chats", CHAT_ID, "debug-runs"],
				data: [detailProbeSummary],
			},
		],
	},
	beforeEach: () => {
		const pendingRequest = () => new Promise<never>(() => {});
		const getChatDebugRunMock = spyOn(
			API.experimental,
			"getChatDebugRun",
		).mockImplementation(pendingRequest);
		return () => {
			getChatDebugRunMock.mockRestore();
		};
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const user = userEvent.setup();

		const runTrigger = await canvas.findByRole("button", {
			name: /Detail state probe/i,
		});
		await user.click(runTrigger);

		await waitFor(() => {
			expect(canvas.getByText(/Loading run details/i)).toBeVisible();
		});
	},
};

export const RunDetailError: Story = {
	parameters: {
		queries: [
			{
				key: ["chats", CHAT_ID, "debug-runs"],
				data: [detailProbeSummary],
			},
		],
	},
	beforeEach: () => {
		const getChatDebugRunMock = spyOn(
			API.experimental,
			"getChatDebugRun",
		).mockRejectedValue(new Error("Unable to fetch run detail"));
		return () => {
			getChatDebugRunMock.mockRestore();
		};
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const user = userEvent.setup();

		const runTrigger = await canvas.findByRole("button", {
			name: /Detail state probe/i,
		});
		await user.click(runTrigger);

		await waitFor(() => {
			expect(canvas.getByText(/Unable to fetch run detail/i)).toBeVisible();
		});
	},
};

export const RunWithNoSteps: Story = {
	parameters: {
		queries: [
			{
				key: ["chats", CHAT_ID, "debug-runs"],
				data: [detailProbeSummary],
			},
			{
				key: ["chats", CHAT_ID, "debug-runs", detailProbeRunId],
				data: makeRun({
					id: detailProbeRunId,
					summary: { first_message: "Detail state probe" },
					steps: [],
				}),
			},
		],
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const user = userEvent.setup();

		const runTrigger = await canvas.findByRole("button", {
			name: /Detail state probe/i,
		});
		await user.click(runTrigger);

		await waitFor(() => {
			expect(canvas.getByText(/No steps recorded/i)).toBeVisible();
		});
	},
};

// ---------------------------------------------------------------------------
// Core state stories.
// ---------------------------------------------------------------------------

export const SingleStepSuccessfulRun: Story = {
	parameters: {
		queries: [
			{
				key: ["chats", CHAT_ID, "debug-runs"],
				data: [
					makeRunSummary({
						id: successfulRunDetail.id,
						summary: successfulRunDetail.summary,
					}),
				],
			},
			{
				key: ["chats", CHAT_ID, "debug-runs", successfulRunDetail.id],
				data: successfulRunDetail,
			},
		],
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const user = userEvent.setup();

		// Expand the run and open the first step before checking nested
		// content.
		const runTrigger = await canvas.findByRole("button", {
			name: /Chat Turn/i,
		});
		await user.click(runTrigger);
		await expandStep(canvas, user);

		await waitFor(() => {
			expect(canvas.getByText("Step 1")).toBeVisible();
			expect(canvas.getAllByText(/^Input$/)[0]).toBeVisible();
			expect(canvas.getAllByText(/^Output$/)[0]).toBeVisible();
		});

		// Request body toggle should be available once the step is open.
		expect(canvas.getByText("Request body")).toBeVisible();

		// Verify a copy button is reachable for normalized body sections.
		await user.click(canvas.getByText("Request body"));
		await waitFor(() => {
			expect(
				canvas.getByRole("button", { name: /Copy request body JSON/i }),
			).toBeVisible();
		});
	},
};

export const MultiStepRunWithRetries: Story = {
	parameters: {
		queries: [
			{
				key: ["chats", CHAT_ID, "debug-runs"],
				data: [
					makeRunSummary({
						id: multiStepRunDetail.id,
						status: multiStepRunDetail.status,
						summary: multiStepRunDetail.summary,
						started_at: multiStepRunDetail.started_at,
						updated_at: multiStepRunDetail.updated_at,
						finished_at: multiStepRunDetail.finished_at,
					}),
				],
			},
			{
				key: ["chats", CHAT_ID, "debug-runs", multiStepRunDetail.id],
				data: multiStepRunDetail,
			},
		],
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const user = userEvent.setup();

		await user.click(await canvas.findByRole("button", { name: /Chat Turn/i }));

		// Both steps render as collapsed headers after the run expands.
		await waitFor(() => {
			expect(canvas.getByText("Step 1")).toBeVisible();
			expect(canvas.getByText("Step 2")).toBeVisible();
		});
		await expandStep(canvas, user);

		// Open Step 1 before asserting on its raw attempt content.
		await waitFor(() => {
			expect(canvas.getByText(/Attempt 1/)).toBeVisible();
			expect(canvas.getByText(/Attempt 2/)).toBeVisible();
			expect(canvas.getByText(/Attempt 3/)).toBeVisible();
		});
	},
};

export const ErrorStateWithRedactedHeaders: Story = {
	parameters: {
		queries: [
			{
				key: ["chats", CHAT_ID, "debug-runs"],
				data: [
					makeRunSummary({
						id: errorRunDetail.id,
						status: errorRunDetail.status,
						summary: errorRunDetail.summary,
						started_at: errorRunDetail.started_at,
						updated_at: errorRunDetail.updated_at,
						finished_at: errorRunDetail.finished_at,
					}),
				],
			},
			{
				key: ["chats", CHAT_ID, "debug-runs", errorRunDetail.id],
				data: errorRunDetail,
			},
		],
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const user = userEvent.setup();

		await user.click(await canvas.findByRole("button", { name: /Chat Turn/i }));
		await expandStep(canvas, user);

		// Open the step before checking the error section and redaction markers.
		// `DebugStepCard` renders `step.error` through `getErrorMessage`, which
		// surfaces `error.message` when present. The fixture's `code`
		// ("upstream_unauthorized") only appears if the message is missing, so
		// assert on the message that the user actually sees.
		await waitFor(() => {
			expect(canvas.getByText(/Provider request failed/i)).toBeVisible();
		});

		// Expand request body to reveal the redacted headers.
		await user.click(canvas.getByText("Request body"));
		await waitFor(() => {
			expect(
				canvas.getByRole("button", { name: /Copy request body JSON/i }),
			).toBeVisible();
		});

		// After expanding, verify [REDACTED] markers appear in the
		// rendered output (Radix Collapsible hides content until open).
		// Use regex since [REDACTED] appears inside larger JSON text
		// nodes, not as standalone text content.
		await waitFor(() => {
			const redactedMarkers = canvas.getAllByText(/\[REDACTED\]/);
			expect(redactedMarkers.length).toBeGreaterThan(0);
		});
	},
};

export const CompactionAndTitleGenerationBadges: Story = {
	parameters: {
		queries: [
			{
				key: ["chats", CHAT_ID, "debug-runs"],
				data: [
					makeRunSummary({
						id: "run-compaction",
						kind: "compaction",
						status: "in_progress",
						provider: "anthropic",
						model: "claude-sonnet-4",
						started_at: "2026-03-05T12:00:03Z",
						updated_at: "2026-03-05T12:00:05Z",
					}),
					makeRunSummary({
						id: "run-chat-turn",
						kind: "chat_turn",
						status: "completed",
						provider: "openai",
						model: "gpt-4.1",
						started_at: "2026-03-05T12:00:01Z",
						updated_at: "2026-03-05T12:00:02Z",
						finished_at: "2026-03-05T12:00:02Z",
					}),
					makeRunSummary({
						id: "run-title",
						kind: "title_generation",
						status: "error",
						provider: "openai",
						model: "gpt-4o-mini",
						started_at: "2026-03-05T12:00:02Z",
						updated_at: "2026-03-05T12:00:04Z",
						finished_at: "2026-03-05T12:00:04Z",
					}),
				],
			},
		],
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		// Verify all three kind badge labels render.
		await expect(canvas.getByText(/compaction/i)).toBeInTheDocument();
		await expect(canvas.getByText(/chat turn/i)).toBeInTheDocument();
		await expect(canvas.getByText(/title generation/i)).toBeInTheDocument();
	},
};

export const LongRawPayloads: Story = {
	parameters: {
		queries: [
			{
				key: ["chats", CHAT_ID, "debug-runs"],
				data: [
					makeRunSummary({
						id: longPayloadRunDetail.id,
						summary: longPayloadRunDetail.summary,
						started_at: longPayloadRunDetail.started_at,
						updated_at: longPayloadRunDetail.updated_at,
						finished_at: longPayloadRunDetail.finished_at,
					}),
				],
			},
			{
				key: ["chats", CHAT_ID, "debug-runs", longPayloadRunDetail.id],
				data: longPayloadRunDetail,
			},
		],
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const user = userEvent.setup();

		await user.click(await canvas.findByRole("button", { name: /Chat Turn/i }));
		await expandStep(canvas, user);

		await waitFor(() => {
			expect(canvas.getByText("Request body")).toBeVisible();
		});

		// Expand request body to see large payloads.
		await user.click(canvas.getByText("Request body"));
		await waitFor(() => {
			expect(canvas.getByText(/request_24/i)).toBeVisible();
		});
	},
};

// ---------------------------------------------------------------------------
// Payload-specific stories.
// ---------------------------------------------------------------------------

export const RichPayloadWithTranscript: Story = {
	parameters: {
		queries: [
			{
				key: ["chats", CHAT_ID, "debug-runs"],
				data: [
					makeRunSummary({
						id: richRunDetail.id,
						summary: richRunDetail.summary,
					}),
				],
			},
			{
				key: ["chats", CHAT_ID, "debug-runs", richRunDetail.id],
				data: richRunDetail,
			},
		],
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const user = userEvent.setup();

		// Run header should show the firstMessage as primary label.
		const runTrigger = await canvas.findByRole("button", {
			name: /Write me a hello world function/i,
		});
		await user.click(runTrigger);
		const stepTrigger = await expandStep(canvas, user);

		await waitFor(() => {
			expect(canvas.getByText("system")).toBeVisible();
			expect(canvas.getByText("user")).toBeVisible();
		});

		// Message content is rendered.
		expect(
			canvas.getByText(/You are a helpful coding assistant/),
		).toBeVisible();
		expect(
			canvas.getAllByText(/Write me a hello world function in Python/)[0],
		).toBeVisible();

		// Output section shows response content.
		expect(canvas.getByText(/Hello, world!/)).toBeVisible();

		// The compact step header keeps model/tokens inline and omits the
		// operation label.
		expect(stepTrigger).toHaveTextContent(/gpt-4/i);
		expect(stepTrigger).toHaveTextContent("150→42 tok");
		expect(stepTrigger).not.toHaveTextContent(/LLM Call/i);

		// Pill toggles for Tools and Options are present.
		const toolsButton = canvas.getByRole("button", { name: /Tools/i });
		expect(toolsButton).toBeVisible();
		await user.click(toolsButton);

		await waitFor(() => {
			expect(canvas.getByText("run_code")).toBeVisible();
			expect(canvas.getByText("search_docs")).toBeVisible();
		});

		// Toggle Options.
		const optionsButton = canvas.getByRole("button", { name: /Options/i });
		await user.click(optionsButton);
		await waitFor(() => {
			expect(canvas.getByText("temperature")).toBeVisible();
		});

		// Toggle Usage.
		const usageButton = canvas.getByRole("button", { name: /Usage/i });
		await user.click(usageButton);
		await waitFor(() => {
			expect(canvas.getByText("prompt_tokens")).toBeVisible();
		});
	},
};

export const ToolCallStep: Story = {
	parameters: {
		queries: [
			{
				key: ["chats", CHAT_ID, "debug-runs"],
				data: [
					makeRunSummary({
						id: toolCallRunDetail.id,
						summary: toolCallRunDetail.summary,
					}),
				],
			},
			{
				key: ["chats", CHAT_ID, "debug-runs", toolCallRunDetail.id],
				data: toolCallRunDetail,
			},
		],
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const user = userEvent.setup();

		await user.click(
			await canvas.findByRole("button", { name: /Run some code/i }),
		);
		await expandStep(canvas, user);

		// Open the step before checking the tool call output.
		await waitFor(() => {
			expect(canvas.getByText("run_code")).toBeVisible();
		});

		// Finish reason shown.
		expect(canvas.getByText(/tool_calls/)).toBeVisible();
	},
};

export const FallbackLabeledRun: Story = {
	parameters: {
		queries: [
			{
				key: ["chats", CHAT_ID, "debug-runs"],
				data: [
					makeRunSummary({
						id: "run-fallback",
						summary: {},
						provider: "anthropic",
						model: "claude-sonnet-4",
					}),
				],
			},
		],
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);

		// Without firstMessage, the run header should fall back to the run kind
		// while keeping the model inline and omitting the provider label.
		const runTrigger = await canvas.findByRole("button", {
			name: /Chat Turn/i,
		});
		expect(runTrigger).toHaveTextContent(/claude-sonnet-4/i);
		expect(runTrigger).not.toHaveTextContent(/Anthropic/i);
	},
};

export const InProgressRun: Story = {
	parameters: {
		queries: [
			{
				key: ["chats", CHAT_ID, "debug-runs"],
				data: [
					makeRunSummary({
						id: "run-progress",
						status: "in_progress",
						provider: "openai",
						model: "gpt-4",
						finished_at: undefined,
					}),
				],
			},
		],
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);

		// The compact run header keeps the model and running status inline.
		const runTrigger = await canvas.findByRole("button", {
			name: /Chat Turn/i,
		});
		expect(runTrigger).toHaveTextContent(/gpt-4/i);
		expect(runTrigger).toHaveTextContent(/in_progress/i);
		expect(runTrigger).not.toHaveTextContent(/Openai/i);
	},
};

// ---------------------------------------------------------------------------
// Backend-normalized shape fixtures (messages with parts, content parts,
// attempts with method/path/status).
// ---------------------------------------------------------------------------

const longToolResultPayload = JSON.stringify({
	value: 4,
	explanation:
		"Explained via calculator tool. ".repeat(24) +
		"The debug panel should clamp this payload until expanded.",
	steps: ["parse expression", "compute result", "return integer"],
});

const backendNormalizedRequest = makeNormalizedPayloadFixture({
	messages: [
		{
			role: "system",
			parts: [
				{
					type: "text",
					text: "You are a calculator. Only output numbers.",
					text_length: 42,
				},
			],
		},
		{
			role: "user",
			parts: [
				{
					type: "text",
					text: "What is 2 + 2?",
					text_length: 14,
				},
			],
		},
		{
			role: "assistant",
			parts: [
				{
					type: "tool-call",
					tool_call_id: "call_abc123",
					tool_name: "calculator",
					arguments: JSON.stringify({ expression: "2 + 2", format: "integer" }),
				},
			],
		},
		{
			role: "tool",
			parts: [
				{
					type: "tool-result",
					tool_call_id: "call_abc123",
					result: longToolResultPayload,
				},
			],
		},
	],
	tools: [
		{
			type: "function",
			name: "calculator",
			description: "Evaluate math",
			input_schema: {
				type: "object",
				properties: {
					expression: { type: "string" },
					format: { type: "string", enum: ["integer", "float"] },
				},
				required: ["expression"],
			},
		},
	],
	options: { max_output_tokens: 128, temperature: 0 },
	tool_choice: "auto",
	provider_option_count: 0,
});

const backendNormalizedResponse = makeNormalizedPayloadFixture({
	content: [
		{
			type: "tool_call",
			tool_call_id: "call_abc123",
			tool_name: "calculator",
			arguments: JSON.stringify({ expression: "2 + 2", format: "integer" }),
		},
	],
	finish_reason: "tool_calls",
	usage: {
		input_tokens: 42,
		output_tokens: 1,
		total_tokens: 43,
		reasoning_tokens: 0,
		cache_creation_tokens: 0,
		cache_read_tokens: 0,
	},
});

const backendNormalizedAttempts = [
	{
		number: 1,
		status: "completed",
		method: "POST",
		url: "https://api.anthropic.com/v1/messages",
		path: "/v1/messages",
		started_at: "2026-03-05T12:00:06Z",
		finished_at: "2026-03-05T12:00:08Z",
		request_headers: { "content-type": "application/json" },
		request_body:
			'{"model":"claude-sonnet-4","messages":[{"role":"user","content":"What is 2 + 2?"}]}',
		response_status: 200,
		response_headers: { "content-type": "application/json" },
		response_body:
			'{"content":[{"type":"text","text":"4"}],"stop_reason":"end_turn"}',
		duration_ms: 1500,
	},
];

const backendShapeRunDetail = makeRun({
	id: "run-backend",
	provider: "anthropic",
	model: "claude-sonnet-4",
	summary: {
		first_message: "What is 2 + 2?",
		endpoint_label: "POST /v1/messages",
		step_count: "1",
		total_input_tokens: "42",
		total_output_tokens: "1",
	},
	steps: [
		makeStep({
			id: "step-backend-1",
			run_id: "run-backend",
			operation: "stream",
			normalized_request: backendNormalizedRequest,
			normalized_response: backendNormalizedResponse,
			usage: {
				input_tokens: "42",
				output_tokens: "1",
				total_tokens: "43",
			},
			attempts: makeAttempts(backendNormalizedAttempts),
		}),
	],
});

export const BackendNormalizedShape: Story = {
	parameters: {
		queries: [
			{
				key: ["chats", CHAT_ID, "debug-runs"],
				data: [
					makeRunSummary({
						id: backendShapeRunDetail.id,
						provider: "anthropic",
						model: "claude-sonnet-4",
						summary: backendShapeRunDetail.summary,
					}),
				],
			},
			{
				key: ["chats", CHAT_ID, "debug-runs", backendShapeRunDetail.id],
				data: backendShapeRunDetail,
			},
		],
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const user = userEvent.setup();

		// Run header should keep the message and model inline, not provider or
		// endpoint labels.
		const runTrigger = await canvas.findByRole("button", {
			name: /What is 2 \+ 2/i,
		});
		expect(runTrigger).toHaveTextContent(/claude-sonnet-4/i);
		expect(runTrigger).not.toHaveTextContent(/Anthropic/i);
		expect(runTrigger).not.toHaveTextContent(/POST \/v1\/messages/i);

		// Expand the run and open the first step before checking transcript
		// content.
		await user.click(runTrigger);
		await expandStep(canvas, user);

		// Only last 2 messages visible by default. The 4-message transcript
		// should be truncated.
		await waitFor(() => {
			expect(canvas.getByText(/Show all 4 messages/)).toBeVisible();
		});

		// Expand transcript to show all messages.
		await user.click(canvas.getByText(/Show all 4 messages/));

		await waitFor(() => {
			expect(canvas.getByText("system")).toBeVisible();
			expect(canvas.getByText("user")).toBeVisible();
		});

		// Verify request message text is visible (not just role badges).
		expect(canvas.getByText(/You are a calculator/)).toBeVisible();
		// "What is 2 + 2?" appears in both the run header and transcript.
		const questionMatches = canvas.getAllByText(/What is 2 \+ 2/);
		expect(questionMatches.length).toBeGreaterThanOrEqual(2);

		// The Tools pill exposes the normalized JSON schema.
		await user.click(canvas.getByRole("button", { name: /Tools/i }));
		await waitFor(() => {
			expect(canvas.getAllByText(/expression/).length).toBeGreaterThan(0);
		});

		// Tool transcript rows are structured cards instead of placeholders.
		expect(canvas.queryByText(/\[tool call:/)).not.toBeInTheDocument();
		expect(canvas.queryByText(/\[tool result:/)).not.toBeInTheDocument();
		await waitFor(() => {
			expect(canvas.getByText(/Explained via calculator tool/)).toBeVisible();
		});
		// Finish reason shown.
		expect(canvas.getByText(/Finish.*tool_calls/)).toBeVisible();

		// Attempt shows method/path and status.
		await waitFor(() => {
			expect(canvas.getByText(/Attempt 1/)).toBeVisible();
		});
		// "POST /v1/messages" now appears only in the attempt header.
		const postMatches = canvas.getAllByText("POST /v1/messages");
		expect(postMatches.length).toBe(1);
		expect(canvas.getAllByText("42→1 tok").length).toBeGreaterThan(0);
		expect(canvas.getByText("200")).toBeVisible();
	},
};
