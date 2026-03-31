import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, screen, waitFor, within } from "storybook/test";
import { StreamingOutput } from "./StreamingOutput";
import {
	buildLiveStatus,
	buildReconnectState,
	buildRetryState,
	FIXTURE_NOW,
} from "./storyFixtures";

// StreamingOutput renders inside a ConversationItem > Message > MessageContent
// chain, but it's self-contained enough to render standalone.

const meta: Meta<typeof StreamingOutput> = {
	title: "pages/AgentsPage/ChatConversation/StreamingOutput",
	component: StreamingOutput,
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
type Story = StoryObj<typeof StreamingOutput>;

/** Default shimmer placeholder with no stream state. */
export const ThinkingPlaceholder: Story = {
	args: {
		streamState: null,
		streamTools: [],
		liveStatus: buildLiveStatus({ isAwaitingFirstStreamChunk: true }),
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const matches = canvas.getAllByText("Thinking...");
		expect(matches.length).toBeGreaterThanOrEqual(1);
		expect(
			canvas.queryByRole("heading", { name: /retrying request/i }),
		).not.toBeInTheDocument();
	},
};

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
		const thinkingMatches = canvas.getAllByText(/thinking\.\.\./i);
		expect(thinkingMatches.length).toBeGreaterThanOrEqual(1);
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
		expect(canvas.getByText("Unexpected error")).toBeVisible();
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
		expect(canvas.getByText("Rate limit")).toBeVisible();
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
		expect(canvas.getByText("Rate limit")).toBeVisible();
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
		expect(canvas.getByText("Overloaded")).toBeVisible();
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
		expect(canvas.getByText("Timeout")).toBeVisible();
		expect(
			canvas.queryByRole("link", { name: /status/i }),
		).not.toBeInTheDocument();
		expect(canvas.queryByText(/provider anthropic/i)).not.toBeInTheDocument();
	},
};

/** Startup timeouts explain the first-token delay before retrying. */
export const RetryStartupTimeout: Story = {
	args: {
		streamState: null,
		streamTools: [],
		liveStatus: buildLiveStatus({
			retryState: buildRetryState({
				kind: "startup_timeout",
				error: "Anthropic did not start responding in time.",
			}),
			isAwaitingFirstStreamChunk: true,
		}),
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(
			canvas.getByRole("heading", { name: /startup timed out/i }),
		).toBeVisible();
		expect(
			canvas.getByText(/anthropic did not start responding in time/i),
		).toBeVisible();
		expect(canvas.getByText("Startup timeout")).toBeVisible();
		expect(canvas.queryByText(/please try again/i)).not.toBeInTheDocument();
		expect(canvas.queryByText(/provider anthropic/i)).not.toBeInTheDocument();
		expect(
			canvas.queryByRole("link", { name: /status/i }),
		).not.toBeInTheDocument();
	},
};
