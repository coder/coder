import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, screen, within } from "storybook/test";
import { AssistantOutput } from "./AssistantOutput";
import {
	buildLiveStatus,
	buildReconnectState,
	buildRetryState,
	buildStreamRenderState,
	pinFixtureClock,
	type StoryStreamRenderState,
} from "./storyFixtures";

// Mirrors how ConversationTimeline normalizes a live row before handing it to
// AssistantOutput.
const LiveAssistantOutput = ({
	streamState,
	streamTools,
	liveStatus,
}: StoryStreamRenderState) => (
	<AssistantOutput
		keyPrefix="stream"
		blocks={streamState?.blocks ?? []}
		tools={streamTools}
		isStreaming={liveStatus.phase === "streaming"}
		liveStatus={liveStatus}
	/>
);

const meta: Meta<typeof LiveAssistantOutput> = {
	title: "pages/AgentsPage/ChatConversation/AssistantOutput",
	component: LiveAssistantOutput,
	beforeEach: pinFixtureClock,
};
export default meta;
type Story = StoryObj<typeof LiveAssistantOutput>;

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
			}),
			isAwaitingFirstStreamChunk: true,
		}),
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
				retryingAt: "not-a-date",
			}),
			isAwaitingFirstStreamChunk: true,
		}),
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
};

export const ResponseDoesNotRenderActivitySlot: Story = {
	args: responseStreamState,
};

/** Tool-only streams use running tool affordances instead of generic thinking. */
export const RunningToolsSuppressThinkingActivity: Story = {
	args: buildStreamRenderState([
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

/** Empty result deltas should not create an invisible completed tool result. */
export const EditFilesEmptyDeltaKeepsRunningHeight: Story = {
	render: () => {
		return (
			<div className="flex flex-col gap-2">
				<div data-testid="running-edit-files">
					<LiveAssistantOutput {...editFilesRunningState} />
				</div>
				<div data-testid="empty-delta-edit-files">
					<LiveAssistantOutput {...editFilesEmptyDeltaState} />
				</div>
			</div>
		);
	},
};
