import { act, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { createRef } from "react";
import { type QueryClient, QueryClientProvider } from "react-query";
import { defaultUrlTransform } from "streamdown";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { API } from "#/api/api";
import { chatPromptsKey } from "#/api/queries/chats";
import type * as TypesGen from "#/api/typesGenerated";
import { MockChat } from "#/testHelpers/chatEntities";
import { createDeferred } from "#/testHelpers/deferred";
import {
	createPreferenceSeededQueryClient,
	renderComponent,
	renderWithAuth,
} from "#/testHelpers/renderHelpers";
import { MockSkill } from "#/testHelpers/skills";
import { MessageScroller } from "#/vendor/message-scroller";
import type { ChatMessageInputRef } from "./AgentChatInput";
import { createChatStore } from "./ChatConversation/chatStore";
import {
	MockAsyncQuestionMessages,
	MockAsyncQuestionResult,
	MockAsyncQuestionResultMessage,
	MockQuestionCallMessage,
	MockQueuedQuestionAnswer,
	workingFixtureTime,
} from "./ChatConversation/storyFixtures";
import { getAsyncAnswerPrefix } from "./ChatElements/tools/asyncAnswer";
import { ChatPageInput, ChatPageTimeline } from "./ChatPageContent";

const renderChatPageInput = (
	store: ReturnType<typeof createChatStore>,
	overrides: Partial<React.ComponentProps<typeof ChatPageInput>> = {},
) =>
	renderWithAuth(
		<ChatPageInput
			chat={{ ...MockChat, id: "", organization_id: "" }}
			store={store}
			models={[]}
			onSend={vi.fn()}
			onDeleteQueuedMessage={vi.fn()}
			onPromoteQueuedMessage={vi.fn()}
			onInterrupt={vi.fn()}
			isInputDisabled={false}
			isSendPending={false}
			isInterruptPending={false}
			hasModelOptions
			selectedModel="model-config-1"
			onModelChange={vi.fn()}
			modelOptions={[
				{
					id: "model-config-1",
					provider: "openai",
					model: "gpt-4o",
					displayName: "GPT-4o",
				},
			]}
			modelSelectorPlaceholder="Select model"
			canConfigureAgentSetup={false}
			isEditing={false}
			onCancelHistoryEdit={vi.fn()}
			isReadOnly={false}
			onReasoningEffortChange={vi.fn()}
			unsupportedProviderNames={[]}
			onPlanModeToggle={vi.fn()}
			isModelCatalogLoading={false}
			inputRef={{ current: null }}
			initialValue=""
			remountKey={0}
			onContentChange={vi.fn()}
			editingFileBlocks={[]}
			mcpServers={[]}
			selectedMCPServerIds={[]}
			onMCPSelectionChange={vi.fn()}
			onMCPAuthComplete={vi.fn()}
			isWorkspaceLoading={false}
			{...overrides}
		/>,
	);

const workspaceFileReference = (
	name: string,
	workspaceId: string,
): TypesGen.ChatWorkspaceFileReferencePart => ({
	type: "workspace-file-reference",
	workspace_file_path: `/home/coder/${name}`,
	workspace_file_name: name,
	workspace_file_size: 42,
	workspace_file_workspace_id: workspaceId,
	workspace_file_media_type: "text/csv",
});

const mockProjectChat: TypesGen.Chat = {
	...MockChat,
	id: "project-chat",
	organization_id: "org-1",
	project_id: "project-1",
};

const mockSkillRequests = () => {
	vi.spyOn(API, "getExperiments").mockResolvedValue(["chat-projects"]);
	vi.spyOn(API.experimental, "getUserSkills").mockResolvedValue([
		{ ...MockSkill, name: "compactor" },
	]);
	vi.spyOn(API.experimental, "getOrganizationSkills").mockResolvedValue([]);
	return vi
		.spyOn(API.experimental, "getChatProjectSkills")
		.mockResolvedValue([]);
};

beforeEach(() => {
	// jsdom lacks Range geometry, which the slash menu reads to anchor.
	// The method does not exist to spy on, so add it per test and remove
	// it afterwards.
	Range.prototype.getBoundingClientRect = () => new DOMRect(0, 0, 1, 16);
});

afterEach(() => {
	vi.restoreAllMocks();
	Reflect.deleteProperty(Range.prototype, "getBoundingClientRect");
});

describe("ChatPageInput", () => {
	it("inserts a project action from More into the chat's composer", async () => {
		const user = userEvent.setup();
		const getChatProjectSkills = mockSkillRequests();
		getChatProjectSkills.mockResolvedValue(
			["deploy", "lint", "release", "triage"].map((name) => ({
				...MockSkill,
				id: `skill-${name}`,
				name,
				action: true,
			})),
		);
		const inputRef = createRef<ChatMessageInputRef>();

		renderChatPageInput(createChatStore(), { chat: mockProjectChat, inputRef });

		await user.click(
			await screen.findByRole("button", { name: "More project actions" }),
		);
		await user.click(await screen.findByRole("menuitem", { name: "triage" }));

		await waitFor(() =>
			expect(inputRef.current?.getContentParts()).toContainEqual({
				type: "skill-invocation",
				source: "project",
				name: "triage",
			}),
		);
		expect(screen.getByRole("textbox", { name: "Chat message" })).toHaveFocus();
	});

	it("routes Stop to onInterrupt while the chat requires action", async () => {
		const user = userEvent.setup();
		const onInterrupt = vi.fn();
		const store = createChatStore();
		store.setChatStatus("requires_action");

		renderChatPageInput(store, { onInterrupt });

		await user.click(await screen.findByRole("button", { name: "Stop" }));
		expect(onInterrupt).toHaveBeenCalledTimes(1);
	});

	it("leaves async question answers out of the prompt history", async () => {
		const user = userEvent.setup();
		vi.spyOn(API.experimental, "getChatPrompts").mockResolvedValue({
			prompts: [
				{ id: 2, text: `${getAsyncAnswerPrefix("call-1")}Frontend` },
				{ id: 1, text: "Build the settings page" },
			],
		});
		const inputRef = createRef<ChatMessageInputRef>();
		const { queryClient } = renderChatPageInput(createChatStore(), {
			chat: { ...MockChat, id: "chat-1", organization_id: "" },
			inputRef,
		});
		await waitFor(() =>
			expect(queryClient.getQueryData(chatPromptsKey("chat-1"))).toBeDefined(),
		);

		await user.click(screen.getByRole("textbox", { name: "Chat message" }));
		await user.keyboard("{ArrowUp}");

		expect(inputRef.current?.getValue()).toBe("Build the settings page");
	});

	it("rehydrates edited workspace file references only from the bound workspace", async () => {
		const user = userEvent.setup();
		const onSend = vi.fn();

		renderChatPageInput(createChatStore(), {
			chat: {
				...MockChat,
				id: "",
				organization_id: "",
				workspace_id: "ws-1",
			},
			onSend,
			isEditing: true,
			initialValue: "edited",
			editingFileBlocks: [
				workspaceFileReference("current.csv", "ws-1"),
				workspaceFileReference("other.csv", "ws-2"),
				workspaceFileReference("unbound.csv", ""),
			],
		});

		await user.click(await screen.findByRole("button", { name: "Save Edit" }));

		await waitFor(() => expect(onSend).toHaveBeenCalledTimes(1));
		expect(onSend.mock.calls[0][0].workspaceUploads).toEqual([
			{
				path: "/home/coder/current.csv",
				name: "current.csv",
				size: 42,
				mediaType: "text/csv",
				workspaceId: "ws-1",
			},
		]);
	});
});

type AnswerCallback = (message: string) => Promise<void> | void;

type TimelineStageProps = {
	store: ReturnType<typeof createChatStore>;
	queryClient: QueryClient;
	onSendAskUserQuestionResponse: AnswerCallback | undefined;
};

const TimelineStage: React.FC<TimelineStageProps> = ({
	store,
	queryClient,
	onSendAskUserQuestionResponse,
}) => (
	<QueryClientProvider client={queryClient}>
		<MessageScroller.Provider autoScroll defaultScrollPosition="end">
			<ChatPageTimeline
				organizationId="organization-id"
				store={store}
				persistedError={undefined}
				hasMoreMessages={false}
				isFetchingMoreMessages={false}
				isHydratingMessages={false}
				hasFetchMoreError={false}
				onFetchMoreMessages={async () => {}}
				urlTransform={defaultUrlTransform}
				mcpServers={[]}
				onSendAskUserQuestionResponse={onSendAskUserQuestionResponse}
				isReadOnly={false}
			/>
		</MessageScroller.Provider>
	</QueryClientProvider>
);

const renderChatPageTimeline = (
	onSendAskUserQuestionResponse: AnswerCallback,
) => {
	const store = createChatStore();
	store.replaceMessages(MockAsyncQuestionMessages);
	const queryClient = createPreferenceSeededQueryClient();
	const { rerender } = renderComponent(
		<TimelineStage
			store={store}
			queryClient={queryClient}
			onSendAskUserQuestionResponse={onSendAskUserQuestionResponse}
		/>,
	);
	return {
		store,
		rerenderWith: (
			nextOnSendAskUserQuestionResponse: AnswerCallback | undefined,
		) =>
			rerender(
				<TimelineStage
					store={store}
					queryClient={queryClient}
					onSendAskUserQuestionResponse={nextOnSendAskUserQuestionResponse}
				/>,
			),
	};
};

const getQuestionDock = () =>
	screen.getByRole("region", { name: "Open question" });

describe("ChatPageTimeline question dock", () => {
	it("answers the open async question from the dock above the input", async () => {
		const user = userEvent.setup();
		const onSendAskUserQuestionResponse = vi.fn();
		renderChatPageTimeline(onSendAskUserQuestionResponse);

		const dock = getQuestionDock();
		await user.click(within(dock).getByRole("radio", { name: /^Frontend/ }));
		await user.click(within(dock).getByRole("button", { name: "Submit" }));

		expect(onSendAskUserQuestionResponse).toHaveBeenCalledWith(
			"Answer to ask_user_question call question:\nFrontend",
		);
	});

	it("announces the docked question once, from its timeline card", () => {
		renderChatPageTimeline(vi.fn());

		const announcements = screen
			.getAllByRole("status")
			.filter((region) =>
				region.textContent?.includes("The agent asked a question"),
			);
		expect(announcements).toHaveLength(1);
		expect(getQuestionDock().contains(announcements[0])).toBe(false);
	});

	it("keeps the dock's draft through a failed send", async () => {
		const user = userEvent.setup();
		const pendingSend = createDeferred<undefined>();
		const onSendAskUserQuestionResponse = vi
			.fn<AnswerCallback>()
			.mockReturnValueOnce(pendingSend.promise);
		const { rerenderWith } = renderChatPageTimeline(
			onSendAskUserQuestionResponse,
		);

		await user.click(
			within(getQuestionDock()).getByRole("radio", { name: /^Frontend/ }),
		);
		await user.click(
			within(getQuestionDock()).getByRole("button", { name: "Submit" }),
		);
		// The page withholds the callback while any send is pending.
		rerenderWith(undefined);
		await act(async () => pendingSend.reject(new Error("Failed to send.")));
		rerenderWith(onSendAskUserQuestionResponse);
		await user.click(
			within(getQuestionDock()).getByRole("button", { name: "Submit" }),
		);

		expect(onSendAskUserQuestionResponse).toHaveBeenCalledTimes(2);
		expect(onSendAskUserQuestionResponse).toHaveBeenNthCalledWith(
			2,
			"Answer to ask_user_question call question:\nFrontend",
		);
	});

	it("keeps the dock's draft when the queue drops its answer", async () => {
		const user = userEvent.setup();
		const onSendAskUserQuestionResponse = vi.fn();
		const { store } = renderChatPageTimeline(onSendAskUserQuestionResponse);

		await user.click(
			within(getQuestionDock()).getByRole("radio", { name: /^Frontend/ }),
		);
		await user.click(
			within(getQuestionDock()).getByRole("button", { name: "Submit" }),
		);
		act(() => {
			store.setQueuedMessages([MockQueuedQuestionAnswer]);
		});
		act(() => {
			store.setQueuedMessages([]);
		});
		await user.click(
			within(getQuestionDock()).getByRole("button", { name: "Submit" }),
		);

		expect(onSendAskUserQuestionResponse).toHaveBeenCalledTimes(2);
		expect(onSendAskUserQuestionResponse).toHaveBeenNthCalledWith(
			2,
			"Answer to ask_user_question call question:\nFrontend",
		);
	});

	it.each([
		{ callId: "newer-question", callIdKind: "a new call ID" },
		{ callId: "question", callIdKind: "a reused call ID" },
	])(
		"gives a newer async question with $callIdKind a fresh form in the dock",
		async ({ callId }) => {
			const user = userEvent.setup();
			const onSendAskUserQuestionResponse = vi.fn();
			const { store } = renderChatPageTimeline(onSendAskUserQuestionResponse);

			await user.click(
				within(getQuestionDock()).getByRole("radio", { name: /^Frontend/ }),
			);
			act(() => {
				store.upsertDurableMessages([
					{
						...MockQuestionCallMessage,
						id: 6,
						content: [
							{
								type: "tool-call",
								tool_call_id: callId,
								tool_name: "ask_user_question",
								args: {},
								created_at: workingFixtureTime(7),
							},
						],
					},
					{
						...MockAsyncQuestionResultMessage,
						id: 7,
						content: [
							{
								type: "tool-result",
								tool_call_id: callId,
								tool_name: "ask_user_question",
								result: MockAsyncQuestionResult,
							},
						],
					},
				]);
			});
			await user.click(
				within(getQuestionDock()).getByRole("button", { name: "Submit" }),
			);

			expect(onSendAskUserQuestionResponse).toHaveBeenCalledWith(
				`Answer to ask_user_question call ${callId}:\nBackend`,
			);
		},
	);
});
