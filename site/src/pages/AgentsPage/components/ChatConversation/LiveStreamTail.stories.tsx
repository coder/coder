import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, waitFor, within } from "storybook/test";
import { LiveStreamTailContent } from "./LiveStreamTail";
import {
	buildLiveStatus,
	buildReconnectState,
	buildRetryState,
	buildStreamRenderState,
	FIXTURE_NOW,
	textResponseStreamParts,
} from "./storyFixtures";

const retryThenResumedStream = buildStreamRenderState(textResponseStreamParts);

const defaultArgs: React.ComponentProps<typeof LiveStreamTailContent> = {
	isTranscriptEmpty: true,
	streamState: null,
	streamTools: [],
	liveStatus: buildLiveStatus(),
	subagentTitles: new Map(),
	subagentStatusOverrides: new Map(),
};

const meta: Meta<typeof LiveStreamTailContent> = {
	title: "pages/AgentsPage/ChatConversation/LiveStreamTail",
	component: LiveStreamTailContent,
	decorators: [
		(Story) => (
			<div className="mx-auto w-full max-w-3xl py-6">
				<Story />
			</div>
		),
	],
	beforeEach: () => {
		const real = Date.now;
		Date.now = () => FIXTURE_NOW;
		return () => {
			Date.now = real;
		};
	},
};
export default meta;
type Story = StoryObj<typeof LiveStreamTailContent>;

/** Empty transcripts show the standard prompt when there is no live tail. */
export const EmptyConversationPrompt: Story = {
	args: defaultArgs,
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(
			canvas.getByText(/start a conversation with your agent/i),
		).toBeVisible();
	},
};

/** Usage-limit failures replace the idle prompt with the analytics CTA. */
export const UsageLimitExceeded: Story = {
	args: {
		...defaultArgs,
		liveStatus: buildLiveStatus({
			persistedError: {
				kind: "usage_limit",
				message:
					"You've used $50.00 of your $50.00 spend limit. Your limit resets on July 1, 2025.",
			},
		}),
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(canvas.getByText(/spend limit/i)).toBeVisible();
		const link = canvas.getByRole("link", { name: /view usage/i });
		expect(link).toBeVisible();
		expect(link).toHaveAttribute("href", "/agents/analytics");
	},
};

/** Provider failures keep the footer-level terminal callout and status link. */
export const TerminalOverloadedError: Story = {
	args: {
		...defaultArgs,
		liveStatus: buildLiveStatus({
			persistedError: {
				kind: "overloaded",
				message: "Anthropic is temporarily overloaded (HTTP 529).",
				provider: "anthropic",
				retryable: true,
				statusCode: 529,
			},
		}),
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(
			canvas.getByRole("heading", { name: /service overloaded/i }),
		).toBeVisible();
		expect(
			canvas.getByText(/anthropic is temporarily overloaded \(http 529\)/i),
		).toBeVisible();
		expect(canvas.queryByText(/please try again/i)).not.toBeInTheDocument();
		expect(canvas.queryByText(/^retryable$/i)).not.toBeInTheDocument();
		expect(canvas.getByRole("link", { name: /status/i })).toBeVisible();
		expect(canvas.queryByText(/provider anthropic/i)).not.toBeInTheDocument();
	},
};

/**
 * Transport timeouts render the per-provider "temporarily
 * unavailable" copy with a "Request timed out" heading rather than
 * the generic "Request failed" fallback.
 */
export const TerminalTimeoutErrorAnthropic: Story = {
	args: {
		...defaultArgs,
		liveStatus: buildLiveStatus({
			streamError: {
				kind: "timeout",
				message: "Anthropic is temporarily unavailable.",
				provider: "anthropic",
				retryable: false,
			},
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
		// Guard against the pre-fix generic fallback.
		expect(
			canvas.queryByText(/the chat request failed unexpectedly/i),
		).not.toBeInTheDocument();
	},
};

/** Transport timeout with an unknown provider uses the generic subject. */
export const TerminalTimeoutErrorUnknownProvider: Story = {
	args: {
		...defaultArgs,
		liveStatus: buildLiveStatus({
			streamError: {
				kind: "timeout",
				message: "The AI provider is temporarily unavailable.",
				retryable: false,
			},
		}),
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(
			canvas.getByRole("heading", { name: /request timed out/i }),
		).toBeVisible();
		expect(
			canvas.getByText(/the ai provider is temporarily unavailable/i),
		).toBeVisible();
	},
};

/** Retrying a transport timeout shows attempt + countdown. */
export const RetryingTimeoutAnthropic: Story = {
	args: {
		...defaultArgs,
		liveStatus: buildLiveStatus({
			retryState: buildRetryState({
				attempt: 2,
				kind: "timeout",
				error: "Anthropic is temporarily unavailable.",
				provider: "anthropic",
			}),
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
		expect(canvas.getByText(/attempt 2/i)).toBeVisible();
		// StatusCountdown renders label and seconds as separate text
		// nodes, so match against the element's combined textContent.
		await waitFor(() => {
			expect(canvasElement).toHaveTextContent(/retrying in \d+s/i);
		});
	},
};

/** Terminal startup timeouts get a specific heading without provider metadata. */
export const TerminalStartupTimeoutError: Story = {
	args: {
		...defaultArgs,
		liveStatus: buildLiveStatus({
			persistedError: {
				kind: "startup_timeout",
				message: "Anthropic did not start responding in time.",
				provider: "anthropic",
				retryable: true,
			},
		}),
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(
			canvas.getByRole("heading", { name: /startup timed out/i }),
		).toBeVisible();
		expect(
			canvas.getByText(/anthropic did not start responding in time./i),
		).toBeVisible();
		expect(canvas.queryByText(/please try again/i)).not.toBeInTheDocument();
		expect(canvas.queryByText(/^retryable$/i)).not.toBeInTheDocument();
		expect(
			canvas.queryByRole("link", { name: /status/i }),
		).not.toBeInTheDocument();
		expect(canvas.queryByText(/provider anthropic/i)).not.toBeInTheDocument();
	},
};

/** Generic failures do not show usage or provider CTAs. */
export const GenericErrorDoesNotShowUsageAction: Story = {
	args: {
		...defaultArgs,
		liveStatus: buildLiveStatus({
			persistedError: {
				kind: "generic",
				message: "Provider request failed.",
			},
		}),
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(
			canvas.getByRole("heading", { name: /request failed/i }),
		).toBeVisible();
		expect(canvas.getByText(/provider request failed/i)).toBeVisible();
		expect(
			canvas.queryByText(/start a conversation with your agent/i),
		).not.toBeInTheDocument();
		expect(
			canvas.queryByRole("link", { name: /view usage/i }),
		).not.toBeInTheDocument();
		expect(
			canvas.queryByRole("link", { name: /status/i }),
		).not.toBeInTheDocument();
	},
};

/** Provider detail renders as a muted secondary line under the main error. */
export const GenericErrorShowsProviderDetail: Story = {
	args: {
		...defaultArgs,
		liveStatus: buildLiveStatus({
			streamError: {
				kind: "generic",
				message: "Anthropic returned an unexpected error (HTTP 400).",
				detail:
					"messages.0.content.1.image.source.base64: image exceeds 5 MB maximum.",
				provider: "anthropic",
				statusCode: 400,
				retryable: false,
			},
		}),
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(
			canvas.getByRole("heading", { name: /request failed/i }),
		).toBeVisible();
		expect(
			canvas.getByText(/anthropic returned an unexpected error \(http 400\)/i),
		).toBeVisible();
		expect(canvas.getByText(/image exceeds 5 mb maximum/i)).toBeVisible();
	},
};

/** Reconnecting keeps already-streamed content visible without a terminal footer. */
export const ReconnectingKeepsPartialOutputVisible: Story = {
	args: {
		...defaultArgs,
		isTranscriptEmpty: false,
		streamState: retryThenResumedStream.streamState,
		streamTools: retryThenResumedStream.streamTools,
		liveStatus: buildLiveStatus({
			streamState: retryThenResumedStream.streamState,
			reconnectState: buildReconnectState({
				attempt: 2,
				delayMs: 2000,
			}),
		}),
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(canvas.getByText(/storybook streamed answer/i)).toBeVisible();
		expect(
			canvas.getByRole("heading", { name: /reconnecting/i }),
		).toBeVisible();
		expect(canvas.getByText(/chat stream disconnected/i)).toBeVisible();
		expect(
			canvas.queryByRole("heading", { name: /request failed/i }),
		).not.toBeInTheDocument();
	},
};

/** Persisted errors yield to live streaming while the live tail is active. */
export const PersistedGenericErrorDoesNotOverrideStreaming: Story = {
	args: {
		...defaultArgs,
		isTranscriptEmpty: false,
		streamState: retryThenResumedStream.streamState,
		streamTools: retryThenResumedStream.streamTools,
		liveStatus: buildLiveStatus({
			streamState: retryThenResumedStream.streamState,
			persistedError: {
				kind: "generic",
				message: "Stale persisted error.",
			},
		}),
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await waitFor(() => {
			expect(canvas.getByText(/storybook streamed answer/i)).toBeVisible();
		});
		expect(
			canvas.queryByRole("heading", { name: /request failed/i }),
		).not.toBeInTheDocument();
	},
};

/** Terminal failures keep partial output visible above the footer callout. */
export const FailedStreamKeepsPartialOutputVisible: Story = {
	args: {
		...defaultArgs,
		isTranscriptEmpty: false,
		streamState: retryThenResumedStream.streamState,
		streamTools: retryThenResumedStream.streamTools,
		liveStatus: buildLiveStatus({
			streamState: retryThenResumedStream.streamState,
			streamError: {
				kind: "generic",
				message: "Provider request failed.",
			},
		}),
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(canvas.getByText(/storybook streamed answer/i)).toBeVisible();
		expect(
			canvas.getByRole("heading", { name: /request failed/i }),
		).toBeVisible();
		expect(canvas.getByText(/provider request failed/i)).toBeVisible();
	},
};
