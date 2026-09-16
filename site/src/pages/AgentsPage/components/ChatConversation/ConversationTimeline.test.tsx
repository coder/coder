import { MessageScroller } from "@shadcn/react/message-scroller";
import { screen, waitForElementToBeRemoved } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClientProvider } from "react-query";
import { describe, expect, it, vi } from "vitest";
import { preferenceSettingsKey } from "#/api/queries/users";
import type { ChatMessage, UserPreferenceSettings } from "#/api/typesGenerated";
import { MockChatMessage } from "#/testHelpers/chatEntities";
import { MockUserPreferenceSettings } from "#/testHelpers/entities";
import {
	createTestQueryClient,
	renderComponent,
} from "#/testHelpers/renderHelpers";
import { ConversationTimeline } from "./ConversationTimeline";
import { parseMessagesWithMergedTools } from "./messageParsing";
import {
	buildWorkingConversation,
	WORKING_FIXTURE_START,
	workingFixtureTime,
} from "./storyFixtures";

const time = workingFixtureTime;
const MockWorkingMessages = buildWorkingConversation();
const MockPreferences: UserPreferenceSettings = {
	...MockUserPreferenceSettings,
	shell_tool_display_mode: "always_collapsed",
	collapse_assistant_steps: true,
};
const MockFailedMessages: ChatMessage[] = MockWorkingMessages.map((message) =>
	message.id === 5
		? {
				...message,
				content: [
					{
						type: "tool-result",
						tool_call_id: "second",
						tool_name: "execute",
						is_error: true,
						result: { output: "Command failed", exit_code: "1" },
						created_at: time(13),
					},
				],
			}
		: message,
);
const MockQuestionMessages: ChatMessage[] = [
	...MockWorkingMessages.slice(0, 3),
	{
		...MockChatMessage,
		id: 4,
		role: "assistant",
		created_at: time(5),
		content: [
			{
				type: "tool-call",
				tool_call_id: "question",
				tool_name: "ask_user_question",
				args: {},
			},
		],
	},
	{
		...MockChatMessage,
		id: 5,
		role: "tool",
		created_at: time(5),
		content: [
			{
				type: "tool-result",
				tool_call_id: "question",
				tool_name: "ask_user_question",
				result: {
					output: JSON.stringify({
						questions: [
							{
								header: "Deploy",
								question: "Deploy the workspace?",
								options: [
									{ label: "Yes", description: "Deploy now" },
									{ label: "No", description: "Keep changes local" },
								],
							},
						],
					}),
				},
			},
		],
	},
];

const longTurnStep = (index: number): ChatMessage[] => [
	{
		...MockChatMessage,
		id: 100 + index * 2,
		role: "assistant",
		created_at: time(index),
		content: [
			{
				type: "tool-call",
				tool_call_id: `step-${index}`,
				tool_name: "execute",
				args: { command: `echo step-${index}` },
				created_at: time(index),
			},
		],
	},
	{
		...MockChatMessage,
		id: 101 + index * 2,
		role: "tool",
		created_at: time(index),
		content: [
			{
				type: "tool-result",
				tool_call_id: `step-${index}`,
				tool_name: "execute",
				result: { output: `step-${index}`, exit_code: "0" },
				created_at: time(index),
			},
		],
	},
];
const MockLongTurn = Array.from({ length: 60 }, (_, index) =>
	longTurnStep(index),
).flat();
const MockLongTurnPrompt: ChatMessage = {
	...MockChatMessage,
	id: 99,
	created_at: time(-1),
	content: [{ type: "text", text: "Run every step" }],
};
const longTurnPages = [
	MockLongTurn.slice(60),
	MockLongTurn.slice(30),
	[MockLongTurnPrompt, ...MockLongTurn],
];

function renderTimeline(
	messages = MockWorkingMessages,
	hasMoreMessages = false,
) {
	const queryClient = createTestQueryClient();
	queryClient.setQueryDefaults(preferenceSettingsKey, {
		staleTime: Number.POSITIVE_INFINITY,
	});
	queryClient.setQueryData(preferenceSettingsKey, MockPreferences);
	const renderMessages = (
		messages: ChatMessage[],
		hasMoreMessages: boolean,
	) => (
		<QueryClientProvider client={queryClient}>
			<MessageScroller.Provider autoScroll defaultScrollPosition="end">
				<MessageScroller.Root>
					<MessageScroller.Viewport>
						<MessageScroller.Content>
							<ConversationTimeline
								organizationId="organization-id"
								subagentTitles={new Map()}
								parsedMessages={parseMessagesWithMergedTools(messages)}
								now={WORKING_FIXTURE_START + 13000}
								hasMoreMessages={hasMoreMessages}
								isChatCompleted
								onSendAskUserQuestionResponse={vi.fn()}
							/>
						</MessageScroller.Content>
					</MessageScroller.Viewport>
				</MessageScroller.Root>
			</MessageScroller.Provider>
		</QueryClientProvider>
	);
	const { rerender } = renderComponent(
		renderMessages(messages, hasMoreMessages),
	);
	return {
		queryClient,
		rerenderMessages: (messages: ChatMessage[], hasMoreMessages: boolean) =>
			rerender(renderMessages(messages, hasMoreMessages)),
	};
}

describe("ConversationTimeline working blocks", () => {
	it("records keyboard expansion changes for a working block", async () => {
		const user = userEvent.setup();
		renderTimeline();
		const summary = screen.getByRole("button", {
			name: "Worked for 12s (2 steps)",
		});
		summary.focus();
		await user.keyboard("{Enter}");
		expect(summary.getAttribute("aria-expanded")).toBe("true");
		await user.keyboard(" ");
		expect(summary.getAttribute("aria-expanded")).toBe("false");
	});

	it("preserves disclosure identity and expansion as older pages join a block", async () => {
		const user = userEvent.setup();
		const { rerenderMessages } = renderTimeline(
			MockWorkingMessages.slice(3),
			true,
		);
		const summary = screen.getByRole("button", {
			name: "Worked for at least 8s (1 step or more)",
		});
		await user.click(summary);
		rerenderMessages(MockWorkingMessages.slice(1), true);
		const grown = screen.getByRole("button", {
			name: "Worked for at least 12s (2 steps or more)",
		});
		expect(grown).toBe(summary);
		expect(grown.getAttribute("aria-expanded")).toBe("true");
		rerenderMessages(MockWorkingMessages, false);
		const complete = screen.getByRole("button", {
			name: "Worked for 12s (2 steps)",
		});
		expect(complete).toBe(summary);
		expect(complete.getAttribute("aria-expanded")).toBe("true");
	});

	it("preserves an existing step node when older rows are prepended", async () => {
		const user = userEvent.setup();
		const { rerenderMessages } = renderTimeline(longTurnPages[0], true);
		await user.click(
			screen.getByRole("button", { name: /Worked for at least/ }),
		);
		rerenderMessages(longTurnPages[1], true);
		const step = screen.getByText(/echo step-15$/);
		rerenderMessages(longTurnPages[2], false);
		expect(screen.getByText(/echo step-15$/)).toBe(step);
	});

	it("restores the previous expansion decision after grouping is reenabled", async () => {
		const user = userEvent.setup();
		const { queryClient } = renderTimeline();
		const summary = screen.getByRole("button", {
			name: "Worked for 12s (2 steps)",
		});
		await user.click(summary);
		queryClient.setQueryData(preferenceSettingsKey, {
			...MockPreferences,
			collapse_assistant_steps: false,
		});
		await waitForElementToBeRemoved(summary);
		queryClient.setQueryData(preferenceSettingsKey, MockPreferences);
		const restored = await screen.findByRole("button", {
			name: "Worked for 12s (2 steps)",
		});
		expect(restored.getAttribute("aria-expanded")).toBe("true");
	});

	it.each([
		{
			scenario: "completed working blocks",
			messages: MockWorkingMessages,
			name: "Worked for 12s (2 steps)",
		},
		{
			scenario: "failed working blocks",
			messages: MockFailedMessages,
			name: /Worked for 12s \(2 steps\)\s*1 failed step/,
		},
		{
			scenario: "the block preceding an interactive question",
			messages: MockQuestionMessages,
			name: "Worked for 3s (1 step)",
		},
	])("starts $scenario collapsed", ({ messages, name }) => {
		renderTimeline(messages);
		const summary = screen.getByRole("button", { name });
		expect(summary.getAttribute("aria-expanded")).toBe("false");
	});
});
