import { MessageScroller } from "@shadcn/react/message-scroller";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { ChatMessageScroller } from "../ChatMessageScroller";
import { TerminalStatusRow } from "./LiveStreamTail";
import { buildLiveStatus, pinFixtureClock } from "./storyFixtures";

const meta: Meta<typeof TerminalStatusRow> = {
	title: "pages/AgentsPage/ChatConversation/TerminalStatusRow",
	component: TerminalStatusRow,
	beforeEach: pinFixtureClock,
	decorators: [
		(Story) => (
			<div className="flex h-[320px] flex-col">
				<MessageScroller.Provider autoScroll defaultScrollPosition="end">
					<ChatMessageScroller
						hasMoreMessages={false}
						isFetchingMoreMessages={false}
						isHydratingMessages={false}
						hasFetchMoreError={false}
						hasTranscriptRows={true}
						onFetchMoreMessages={async () => {}}
					>
						<Story />
					</ChatMessageScroller>
				</MessageScroller.Provider>
			</div>
		),
	],
};
export default meta;
type Story = StoryObj<typeof TerminalStatusRow>;

export const UsageLimitExceeded: Story = {
	args: {
		liveStatus: buildLiveStatus({
			persistedError: {
				kind: "usage_limit",
				message: "Your AI spend budget has been reached.",
			},
		}),
	},
};

export const ProviderQuotaExceeded: Story = {
	args: {
		liveStatus: buildLiveStatus({
			streamError: {
				kind: "usage_limit",
				message:
					"The usage quota for OpenAI has been exceeded. Check the billing and quota settings for the provider account.",
				provider: "openai",
				retryable: false,
			},
		}),
	},
};

/** Provider failures keep the footer-level terminal callout and status link. */

export const TerminalOverloadedError: Story = {
	args: {
		liveStatus: buildLiveStatus({
			persistedError: {
				kind: "overloaded",
				message: "Anthropic is temporarily overloaded.",
				provider: "anthropic",
				retryable: true,
				statusCode: 529,
			},
		}),
	},
};

/** Content-filter refusals render as terminal errors without a retry countdown or status link. */

export const TerminalContentFilterError: Story = {
	args: {
		liveStatus: buildLiveStatus({
			persistedError: {
				kind: "content_filter",
				message:
					"Anthropic blocked this response under its content policy (cyber).",
				detail:
					"This request triggered restrictions on violative cyber content and was blocked under Anthropic's Usage Policy. To learn more, see https://platform.claude.com/docs/en/build-with-claude/refusals-and-fallback.",
				provider: "anthropic",
				retryable: false,
			},
		}),
	},
};

/**
 * Transport timeouts render the per-provider "temporarily
 * unavailable" copy with a "Request timed out" heading rather than
 * the generic "Request failed" fallback.
 */

export const TerminalTimeoutErrorAnthropic: Story = {
	args: {
		liveStatus: buildLiveStatus({
			streamError: {
				kind: "timeout",
				message: "Anthropic is temporarily unavailable.",
				provider: "anthropic",
				retryable: false,
			},
		}),
	},
};

/** Transport timeout with an unknown provider uses the generic subject. */

export const TerminalTimeoutErrorUnknownProvider: Story = {
	args: {
		liveStatus: buildLiveStatus({
			streamError: {
				kind: "timeout",
				message: "The AI provider is temporarily unavailable.",
				retryable: false,
			},
		}),
	},
};

/** Missing API key shows the "Chat interrupted" terminal error. */

export const TerminalMissingKeyError: Story = {
	args: {
		liveStatus: buildLiveStatus({
			streamError: {
				kind: "missing_key",
				message:
					"This conversation was started with an API key that is no longer available. Send your message again to continue.",
				retryable: false,
				detail:
					"If this error persists after resending, please report it as a bug.",
			},
		}),
	},
};

/** Terminal stream-silence timeouts get a specific heading without provider metadata. */

export const TerminalStreamSilenceTimeoutError: Story = {
	args: {
		liveStatus: buildLiveStatus({
			persistedError: {
				kind: "stream_silence_timeout",
				message: "Anthropic did not send response data in time.",
				provider: "anthropic",
				retryable: true,
			},
		}),
	},
};

/** Disabled provider errors render an admin-oriented message without retry. */

export const TerminalProviderDisabledError: Story = {
	args: {
		liveStatus: buildLiveStatus({
			streamError: {
				kind: "provider_disabled",
				message:
					"The OpenAI provider has been disabled. Contact your Coder administrator.",
				provider: "openai",
				retryable: false,
				statusCode: 503,
			},
		}),
	},
};

/** Generic failures do not show usage or provider CTAs. */

export const GenericErrorDoesNotShowUsageAction: Story = {
	args: {
		liveStatus: buildLiveStatus({
			persistedError: {
				kind: "generic",
				message: "Provider request failed.",
			},
		}),
	},
};

/** Provider detail renders in a monospace block for generic errors. */

export const GenericErrorShowsProviderDetail: Story = {
	args: {
		liveStatus: buildLiveStatus({
			streamError: {
				kind: "generic",
				message: "Anthropic returned an unexpected error.",
				detail:
					"messages.0.content.1.image.source.base64: image exceeds 5 MB maximum.",
				provider: "anthropic",
				statusCode: 400,
				retryable: false,
			},
		}),
	},
};
