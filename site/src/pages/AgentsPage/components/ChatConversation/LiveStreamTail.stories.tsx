import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, waitFor, within } from "storybook/test";
import { LiveStreamTailContent } from "./LiveStreamTail";
import {
	buildLiveStatus,
	buildReconnectState,
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
				message: "Anthropic is currently overloaded.",
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
		expect(canvas.getByText("Overloaded")).toBeVisible();
		expect(
			canvas.getByText(/anthropic is currently overloaded./i),
		).toBeVisible();
		expect(canvas.queryByText(/please try again/i)).not.toBeInTheDocument();
		expect(canvas.queryByText(/^retryable$/i)).not.toBeInTheDocument();
		expect(canvas.getByText(/http 529/i)).toBeVisible();
		expect(canvas.getByRole("link", { name: /status/i })).toBeVisible();
		expect(canvas.queryByText(/provider anthropic/i)).not.toBeInTheDocument();
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
		expect(canvas.getByText("Startup timeout")).toBeVisible();
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
