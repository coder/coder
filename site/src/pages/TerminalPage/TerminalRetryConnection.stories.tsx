import { action } from "@storybook/addon-actions";
import type { Meta, StoryObj } from "@storybook/react";
import { TerminalRetryConnection } from "./TerminalRetryConnection";

const meta: Meta<typeof TerminalRetryConnection> = {
	title: "pages/TerminalPage/TerminalRetryConnection",
	component: TerminalRetryConnection,
	parameters: {
		layout: "padded",
	},
	args: {
		onRetryNow: action("onRetryNow"),
		maxAttempts: 10,
	},
};

export default meta;
type Story = StoryObj<typeof TerminalRetryConnection>;

// Hidden state - component returns null
export const Hidden: Story = {
	args: {
		isRetrying: false,
		timeUntilNextRetry: null,
		attemptCount: 0,
	},
};

// Currently retrying state - shows "Reconnecting..." with no button
export const Retrying: Story = {
	args: {
		isRetrying: true,
		timeUntilNextRetry: null,
		attemptCount: 1,
	},
};

// Countdown to next retry - first attempt (1 second)
export const CountdownFirstAttempt: Story = {
	args: {
		isRetrying: false,
		timeUntilNextRetry: 1000, // 1 second
		attemptCount: 1,
	},
};

// Countdown to next retry - second attempt (2 seconds)
export const CountdownSecondAttempt: Story = {
	args: {
		isRetrying: false,
		timeUntilNextRetry: 2000, // 2 seconds
		attemptCount: 2,
	},
};

// Countdown to next retry - longer delay (15 seconds)
export const CountdownLongerDelay: Story = {
	args: {
		isRetrying: false,
		timeUntilNextRetry: 15000, // 15 seconds
		attemptCount: 5,
	},
};

// Countdown with 1 second remaining (singular)
export const CountdownOneSecond: Story = {
	args: {
		isRetrying: false,
		timeUntilNextRetry: 1000, // 1 second
		attemptCount: 3,
	},
};

// Countdown with less than 1 second remaining
export const CountdownLessThanOneSecond: Story = {
	args: {
		isRetrying: false,
		timeUntilNextRetry: 500, // 0.5 seconds (should show "1 second")
		attemptCount: 3,
	},
};

// Max attempts reached - no more automatic retries
export const MaxAttemptsReached: Story = {
	args: {
		isRetrying: false,
		timeUntilNextRetry: null,
		attemptCount: 10,
		maxAttempts: 10,
	},
};

// Connection lost but no retry scheduled yet
export const ConnectionLostNoRetry: Story = {
	args: {
		isRetrying: false,
		timeUntilNextRetry: null,
		attemptCount: 1,
		maxAttempts: 10,
	},
};
