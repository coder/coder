import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, screen, waitFor, within } from "storybook/test";
import { StreamingOutput } from "./StreamingOutput";
import {
	buildLiveStatus,
	buildReconnectState,
	buildRetryState,
	buildStreamRenderState,
	pinFixtureClock,
} from "./storyFixtures";

// StreamingOutput renders inside a ConversationItem > Message > MessageContent
// chain, but it's self-contained enough to render standalone.

const meta: Meta<typeof StreamingOutput> = {
	title: "pages/AgentsPage/ChatConversation/StreamingOutput",
	component: StreamingOutput,
	beforeEach: pinFixtureClock,
};
export default meta;
type Story = StoryObj<typeof StreamingOutput>;

/** Transport reconnects render a non-terminal reconnecting callout. */
export const ReconnectingAfterDisconnect: Story = {
	args: {
		streamState: null,
		streamTools: [],
		liveStatus: buildLiveStatus({
			reconnectState: buildReconnectState({
				attempt: 2,
				delayMs: 2000,
			}),
		}),
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(
			canvas.getByRole("heading", { name: /reconnecting/i }),
		).toBeVisible();
		expect(canvas.getByText(/chat stream disconnected/i)).toBeVisible();
		expect(canvas.getByText(/attempt 2/i)).toBeVisible();
		await waitFor(() => {
			expect(canvasElement.textContent).toMatch(/reconnecting in \d+s/i);
		});
		expect(canvas.queryByText("Unexpected error")).not.toBeInTheDocument();
		expect(canvas.queryByTestId("live-activity-slot")).not.toBeInTheDocument();
		expect(canvas.queryByText("Thinking...")).not.toBeInTheDocument();
	},
};

/** Generic retry reasons show automatic retry copy without a manual CTA. */
export const RetryWithVisibleReason: Story = {
	args: {
		streamState: null,
		streamTools: [],
		liveStatus: buildLiveStatus({
			retryState: buildRetryState(),
			isAwaitingFirstStreamChunk: true,
		}),
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(
			canvas.getByRole("heading", { name: /retrying request/i }),
		).toBeVisible();
		expect(
			canvas.getByText(/anthropic returned an unexpected error/i),
		).toBeVisible();
		expect(canvas.queryByTestId("live-activity-slot")).not.toBeInTheDocument();
		expect(canvas.queryByText("Thinking...")).not.toBeInTheDocument();
		expect(canvas.getByText(/attempt 1/i)).toBeVisible();
		expect(canvas.queryByText(/please try again/i)).not.toBeInTheDocument();
		expect(canvas.queryByText(/provider anthropic/i)).not.toBeInTheDocument();
	},
};

/** Rate-limited retries expose the normalized kind and delay metadata. */
export const RetryRateLimited: Story = {
	args: {
		streamState: null,
		streamTools: [],
		liveStatus: buildLiveStatus({
			retryState: buildRetryState({
				attempt: 3,
				error: "Anthropic is rate limiting requests.",
				kind: "rate_limit",
				delayMs: 3000,
			}),
			isAwaitingFirstStreamChunk: true,
		}),
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(
			canvas.getByRole("heading", { name: /rate limited/i }),
		).toBeVisible();
		expect(
			canvas.getByText(/anthropic is rate limiting requests/i),
		).toBeVisible();
		await waitFor(() => {
			expect(canvasElement.textContent).toMatch(/retrying in \d+s/i);
		});
		expect(
			canvas.queryByRole("link", { name: /status/i }),
		).not.toBeInTheDocument();
		expect(canvas.queryByText(/provider anthropic/i)).not.toBeInTheDocument();
	},
};

/** Invalid retry timestamps hide the countdown instead of rendering NaN. */
export const RetryInvalidTimestamp: Story = {
	args: {
		streamState: null,
		streamTools: [],
		liveStatus: buildLiveStatus({
			retryState: buildRetryState({
				attempt: 3,
				error: "Anthropic is rate limiting requests.",
				kind: "rate_limit",
				delayMs: 3000,
				retryingAt: "not-a-date",
			}),
			isAwaitingFirstStreamChunk: true,
		}),
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(
			canvas.getByRole("heading", { name: /rate limited/i }),
		).toBeVisible();
		expect(
			canvas.getByText(/anthropic is rate limiting requests/i),
		).toBeVisible();
		expect(canvas.getByText(/attempt 3/i)).toBeVisible();
		await waitFor(() => {
			expect(canvas.queryByText(/retrying in nan/i)).not.toBeInTheDocument();
			expect(canvas.queryByText(/retrying in \d+s/i)).not.toBeInTheDocument();
		});
		expect(canvas.queryByText(/provider anthropic/i)).not.toBeInTheDocument();
	},
};

/** Overloaded retries expose provider status links while retrying. */
export const RetryOverloaded: Story = {
	args: {
		streamState: null,
		streamTools: [],
		liveStatus: buildLiveStatus({
			retryState: buildRetryState({
				kind: "overloaded",
				provider: "anthropic",
				error: "Anthropic is temporarily overloaded.",
			}),
			isAwaitingFirstStreamChunk: true,
		}),
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(
			canvas.getByRole("heading", { name: /service overloaded/i }),
		).toBeVisible();
		expect(
			canvas.getByText(/anthropic is temporarily overloaded/i),
		).toBeVisible();
		const statusLink = screen.getByRole("link", { name: /status/i });
		expect(statusLink).toBeVisible();
		expect(statusLink).toHaveAttribute("href", "https://status.anthropic.com");
		expect(canvas.queryByText(/provider anthropic/i)).not.toBeInTheDocument();
	},
};

/** Timeout retries render timeout-specific copy without a status CTA. */
export const RetryTimeout: Story = {
	args: {
		streamState: null,
		streamTools: [],
		liveStatus: buildLiveStatus({
			retryState: buildRetryState({
				kind: "timeout",
				error: "Anthropic is temporarily unavailable.",
			}),
			isAwaitingFirstStreamChunk: true,
		}),
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(
			canvas.getByRole("heading", { name: /request timed out/i }),
		).toBeVisible();
		expect(
			canvas.getByText(/anthropic is temporarily unavailable/i),
		).toBeVisible();
		expect(
			canvas.queryByRole("link", { name: /status/i }),
		).not.toBeInTheDocument();
		expect(canvas.queryByText(/provider anthropic/i)).not.toBeInTheDocument();
	},
};

/** Stream-silence timeouts explain the first-token delay before retrying. */
export const RetryStreamSilenceTimeout: Story = {
	args: {
		streamState: null,
		streamTools: [],
		liveStatus: buildLiveStatus({
			retryState: buildRetryState({
				kind: "stream_silence_timeout",
				error: "Anthropic did not send response data in time.",
			}),
			isAwaitingFirstStreamChunk: true,
		}),
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(
			canvas.getByRole("heading", { name: /response stalled/i }),
		).toBeVisible();
		expect(
			canvas.getByText(/anthropic did not send response data in time/i),
		).toBeVisible();
		expect(canvas.queryByText(/please try again/i)).not.toBeInTheDocument();
		expect(canvas.queryByText(/provider anthropic/i)).not.toBeInTheDocument();
		expect(
			canvas.queryByRole("link", { name: /status/i }),
		).not.toBeInTheDocument();
	},
};

const responseStreamState = buildStreamRenderState([
	{
		type: "text" as const,
		text: "The answer is streaming.",
	},
]);

export const StartingShowsThinkingActivity: Story = {
	args: {
		streamState: null,
		streamTools: [],
		liveStatus: buildLiveStatus({ isAwaitingFirstStreamChunk: true }),
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(canvas.getByText("Thinking")).toBeVisible();
		expect(canvas.getByTestId("live-activity-slot")).toBeVisible();
	},
};

export const ResponseDoesNotRenderActivitySlot: Story = {
	args: {
		streamState: responseStreamState.streamState,
		streamTools: responseStreamState.streamTools,
		liveStatus: responseStreamState.liveStatus,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(canvas.queryByTestId("live-activity-slot")).not.toBeInTheDocument();
	},
};

/** Tool-only streams use running tool affordances instead of generic thinking. */
export const RunningToolsSuppressThinkingActivity: Story = {
	args: {
		...buildStreamRenderState([
			{
				type: "tool-call",
				tool_name: "execute",
				tool_call_id: "tc-1",
				args: { command: "ls -la" },
			},
			{
				type: "tool-call",
				tool_name: "read_file",
				tool_call_id: "tc-2",
				args: { path: "README.md" },
			},
		]),
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(canvas.queryByTestId("live-activity-slot")).not.toBeInTheDocument();
		expect(
			canvas.getByRole("button", { name: /expand command/i }),
		).toBeVisible();
		expect(canvas.getByText(/reading README\.md/i)).toBeVisible();
	},
};

const editFilesArgs = {
	files: JSON.stringify([
		{
			path: "src/config.ts",
			edits: [
				{
					old_text: "const timeout = 30;",
					new_text: "const timeout = 60;",
				},
			],
		},
	]),
};

const editFilesRunningState = buildStreamRenderState([
	{
		type: "tool-call",
		tool_call_id: "edit-tool",
		tool_name: "edit_files",
		args: editFilesArgs,
	},
]);

const editFilesEmptyDeltaState = buildStreamRenderState([
	{
		type: "tool-call",
		tool_call_id: "edit-tool",
		tool_name: "edit_files",
		args: editFilesArgs,
	},
	{
		type: "tool-result",
		tool_call_id: "edit-tool",
		tool_name: "edit_files",
		result_delta: "",
	},
]);

const getEditFilesToolHeight = (canvasElement: HTMLElement) => {
	const editTool = canvasElement.querySelector("[data-transcript-row]");
	expect(editTool).not.toBeNull();
	return Math.round(editTool?.getBoundingClientRect().height ?? 0);
};

/** Empty result deltas should not create an invisible completed tool result. */
export const EditFilesEmptyDeltaKeepsRunningHeight: Story = {
	render: () => {
		return (
			<div className="flex flex-col gap-2">
				<div data-testid="running-edit-files">
					<StreamingOutput
						streamState={editFilesRunningState.streamState}
						streamTools={editFilesRunningState.streamTools}
						liveStatus={editFilesRunningState.liveStatus}
					/>
				</div>
				<div data-testid="empty-delta-edit-files">
					<StreamingOutput
						streamState={editFilesEmptyDeltaState.streamState}
						streamTools={editFilesEmptyDeltaState.streamTools}
						liveStatus={editFilesEmptyDeltaState.liveStatus}
					/>
				</div>
			</div>
		);
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const running = canvas.getByTestId("running-edit-files");
		const emptyDelta = canvas.getByTestId("empty-delta-edit-files");

		expect(within(running).getByText(/Editing files/)).toBeVisible();
		expect(within(emptyDelta).getByText(/Editing files/)).toBeVisible();
		expect(getEditFilesToolHeight(emptyDelta)).toBe(
			getEditFilesToolHeight(running),
		);
	},
};
