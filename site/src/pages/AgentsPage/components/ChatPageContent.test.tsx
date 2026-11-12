import { act, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { createRef } from "react";
import { QueryClientProvider } from "react-query";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { API } from "#/api/api";
import { authorizationKey, getAuthorizationKey } from "#/api/queries/authCheck";
import { preferenceSettingsKey } from "#/api/queries/users";
import type * as TypesGen from "#/api/typesGenerated";
import { MockChat, MockChatQueuedMessage } from "#/testHelpers/chatEntities";
import {
	MockUserChatCompactionThresholds,
	MockUserPreferenceSettings,
} from "#/testHelpers/entities";
import {
	createTestQueryClient,
	renderComponent,
	renderWithAuth,
} from "#/testHelpers/renderHelpers";
import { MockSkill } from "#/testHelpers/skills";
import { MessageScroller } from "#/vendor/message-scroller";
import type { ChatMessageInputRef } from "./AgentChatInput";
import { createChatStore } from "./ChatConversation/chatStore";
import {
	buildQuestionArgs,
	MockAsyncQuestionAnswerMessage,
	MockAsyncQuestionCallMessage,
	MockAsyncQuestionId,
	MockAsyncQuestionMessages,
	MockAsyncQuestionResultMessage,
	MockAsyncQuestions,
	MockNewerAsyncQuestionId,
	MockNewerAsyncQuestionMessages,
	MockQueuedQuestionAnswer,
} from "./ChatConversation/storyFixtures";
import {
	ChatPageInput,
	ChatPageTimeline,
	workspaceSkillsFromChat,
} from "./ChatPageContent";

const renderChatPageTimeline = (
	store: ReturnType<typeof createChatStore>,
	initialProps: Partial<React.ComponentProps<typeof ChatPageTimeline>> = {},
) => {
	const queryClient = createTestQueryClient();
	queryClient.setQueryDefaults(preferenceSettingsKey, {
		staleTime: Number.POSITIVE_INFINITY,
	});
	queryClient.setQueryData(preferenceSettingsKey, MockUserPreferenceSettings);

	const renderStage = (
		props: Partial<React.ComponentProps<typeof ChatPageTimeline>>,
	) => (
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
					onFetchMoreMessages={vi.fn()}
					isReadOnly={false}
					urlTransform={(url) => url}
					mcpServers={[]}
					{...initialProps}
					{...props}
				/>
			</MessageScroller.Provider>
		</QueryClientProvider>
	);
	const result = renderComponent(renderStage({}));

	return {
		rerenderTimeline: (
			props: Partial<React.ComponentProps<typeof ChatPageTimeline>> = {},
		) => result.rerender(renderStage(props)),
	};
};

describe("ChatPageTimeline async questions", () => {
	it("submits the chosen answer with the question ID", async () => {
		const user = userEvent.setup();
		const onSendAskUserQuestionResponse = vi.fn();
		const store = createChatStore();
		store.replaceMessages(MockAsyncQuestionMessages);

		renderChatPageTimeline(store, { onSendAskUserQuestionResponse });

		await user.click(
			await screen.findByRole("radio", { name: /^Frontend first/ }),
		);
		await user.click(screen.getByRole("button", { name: "Submit" }));

		await waitFor(() => {
			expect(onSendAskUserQuestionResponse).toHaveBeenCalledTimes(1);
		});
		expect(onSendAskUserQuestionResponse).toHaveBeenCalledWith(
			"Frontend first, then the API.",
			MockAsyncQuestionId,
		);
	});

	it("answers a blocking question from its card without a question ID", async () => {
		const user = userEvent.setup();
		const onSendAskUserQuestionResponse = vi.fn();
		const store = createChatStore();
		store.replaceMessages([
			...MockAsyncQuestionMessages.slice(0, 1),
			{
				...MockAsyncQuestionCallMessage,
				content: [
					{
						type: "tool-call",
						tool_call_id: "blocking-question",
						tool_name: "ask_user_question",
						args: buildQuestionArgs(MockAsyncQuestions),
					},
				],
			},
			{
				...MockAsyncQuestionResultMessage,
				content: [
					{
						type: "tool-result",
						tool_call_id: "blocking-question",
						tool_name: "ask_user_question",
						result: { questions: MockAsyncQuestions },
					},
				],
			},
		]);

		renderChatPageTimeline(store, { onSendAskUserQuestionResponse });

		await user.click(
			await screen.findByRole("radio", { name: /^Frontend first/ }),
		);
		await user.click(screen.getByRole("button", { name: "Submit" }));

		await waitFor(() => {
			expect(onSendAskUserQuestionResponse).toHaveBeenCalledTimes(1);
		});
		expect(onSendAskUserQuestionResponse).toHaveBeenCalledWith(
			"Frontend first, then the API.",
		);
	});

	it("keeps the selected answer when a queued answer is removed", async () => {
		const user = userEvent.setup();
		const onSendAskUserQuestionResponse = vi.fn().mockResolvedValue(undefined);
		const store = createChatStore();
		store.replaceMessages(MockAsyncQuestionMessages);
		const { rerenderTimeline } = renderChatPageTimeline(store, {
			onSendAskUserQuestionResponse,
		});

		await user.click(
			await screen.findByRole("radio", { name: /^Frontend first/ }),
		);
		await user.click(screen.getByRole("button", { name: "Submit" }));
		await waitFor(() => {
			expect(onSendAskUserQuestionResponse).toHaveBeenCalledTimes(1);
		});

		act(() => {
			store.setQueuedMessages([MockQueuedQuestionAnswer]);
		});
		rerenderTimeline();
		act(() => {
			store.setQueuedMessages([]);
		});
		rerenderTimeline();
		await user.click(await screen.findByRole("button", { name: "Submit" }));

		await waitFor(() => {
			expect(onSendAskUserQuestionResponse).toHaveBeenCalledTimes(2);
		});
		expect(onSendAskUserQuestionResponse).toHaveBeenNthCalledWith(
			2,
			"Frontend first, then the API.",
			MockAsyncQuestionId,
		);
	});

	it("keeps the selected answer while an earlier message is edited", async () => {
		const user = userEvent.setup();
		const onSendAskUserQuestionResponse = vi.fn();
		const store = createChatStore();
		store.replaceMessages(MockAsyncQuestionMessages);
		const { rerenderTimeline } = renderChatPageTimeline(store, {
			onSendAskUserQuestionResponse,
		});

		await user.click(
			await screen.findByRole("radio", { name: /^Frontend first/ }),
		);
		rerenderTimeline({ editingMessageId: 1 });
		rerenderTimeline({ editingMessageId: null });
		await user.click(screen.getByRole("button", { name: "Submit" }));

		await waitFor(() => {
			expect(onSendAskUserQuestionResponse).toHaveBeenCalledWith(
				"Frontend first, then the API.",
				MockAsyncQuestionId,
			);
		});
	});

	it("submits the newer question's answer without sending the earlier selection", async () => {
		const user = userEvent.setup();
		const onSendAskUserQuestionResponse = vi.fn();
		const store = createChatStore();
		store.replaceMessages(MockAsyncQuestionMessages);
		const { rerenderTimeline } = renderChatPageTimeline(store, {
			onSendAskUserQuestionResponse,
		});

		await user.click(
			await screen.findByRole("radio", { name: /^Frontend first/ }),
		);
		act(() => {
			store.replaceMessages(MockNewerAsyncQuestionMessages);
		});
		rerenderTimeline();
		await user.click(screen.getByRole("button", { name: "Submit" }));

		await waitFor(() => {
			expect(onSendAskUserQuestionResponse).toHaveBeenCalledTimes(1);
		});
		expect(onSendAskUserQuestionResponse).toHaveBeenCalledWith(
			"Cover the change with unit tests.",
			MockNewerAsyncQuestionId,
		);
	});

	it("does not announce a question whose questions cannot be read", () => {
		const store = createChatStore();
		store.replaceMessages([MockAsyncQuestionResultMessage]);

		renderChatPageTimeline(store);

		expect(screen.getByRole("status")).toHaveTextContent(/^$/);
	});

	it("announces an open question and clears the announcement after its answer", () => {
		const store = createChatStore();
		store.replaceMessages(MockAsyncQuestionMessages);
		const { rerenderTimeline } = renderChatPageTimeline(store);

		expect(screen.getByRole("status")).toHaveTextContent(
			"The agent asked a question: Which part should change first?",
		);

		act(() => {
			store.replaceMessages([
				...MockAsyncQuestionMessages,
				MockAsyncQuestionAnswerMessage,
			]);
		});
		rerenderTimeline();

		expect(screen.getByRole("status")).toHaveTextContent(/^$/);
	});
});

const skillResource = (
	name: string,
	overrides: Partial<TypesGen.ChatContextResource> = {},
): TypesGen.ChatContextResource => ({
	source: `/workspace/.agents/skills/${name}`,
	kind: "skill",
	size_bytes: 128,
	skill_name: name,
	skill_description: `${name} description`,
	status: "ok",
	...overrides,
});

const instructionResource = (): TypesGen.ChatContextResource => ({
	source: "/workspace/AGENTS.md",
	kind: "instruction_file",
	size_bytes: 64,
	status: "ok",
});

const chatWithContext = (
	context: TypesGen.ChatContext | undefined,
): TypesGen.Chat => ({ ...MockChat, context });

describe("workspaceSkillsFromChat", () => {
	it("returns undefined while the chat detail is unresolved", () => {
		expect(workspaceSkillsFromChat(undefined)).toBeUndefined();
	});

	it("returns an empty authoritative list for a resolved unpinned chat", () => {
		expect(workspaceSkillsFromChat(chatWithContext(undefined))).toEqual([]);
		expect(workspaceSkillsFromChat(chatWithContext({ dirty: false }))).toEqual(
			[],
		);
	});

	it("maps healthy skill resources to workspace skills", () => {
		const chat = chatWithContext({
			dirty: false,
			resources: [
				instructionResource(),
				skillResource("reviewer"),
				skillResource("docs"),
			],
		});
		expect(workspaceSkillsFromChat(chat)).toEqual([
			{ name: "reviewer", description: "reviewer description" },
			{ name: "docs", description: "docs description" },
		]);
	});

	it("keeps the first resource for duplicate skill names, matching read_skill", () => {
		const chat = chatWithContext({
			dirty: false,
			resources: [
				skillResource("reviewer", {
					source: "/workspace/.agents/skills/reviewer",
				}),
				skillResource("reviewer", {
					source: "/workspace/other/skills/reviewer",
					skill_description: "shadowed duplicate",
				}),
			],
		});
		expect(workspaceSkillsFromChat(chat)).toEqual([
			{ name: "reviewer", description: "reviewer description" },
		]);
	});

	it("omits non-ok skill resources", () => {
		const chat = chatWithContext({
			dirty: true,
			resources: [
				skillResource("reviewer"),
				skillResource("broken", { status: "unreadable", skill_name: "" }),
			],
		});
		expect(workspaceSkillsFromChat(chat)).toEqual([
			{ name: "reviewer", description: "reviewer description" },
		]);
	});

	it("returns an empty authoritative list when pinned context has no skills", () => {
		const chat = chatWithContext({
			dirty: false,
			resources: [instructionResource()],
		});
		expect(workspaceSkillsFromChat(chat)).toEqual([]);
	});
});

const renderChatPageInput = (
	store: ReturnType<typeof createChatStore>,
	overrides: Partial<React.ComponentProps<typeof ChatPageInput>> = {},
) =>
	renderWithAuth(
		<ChatPageInput
			chat={{ ...MockChat, id: "", organization_id: "" }}
			store={store}
			modelCatalog={undefined}
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
			editingTarget={null}
			onCancelEdit={vi.fn()}
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
			isWorkspaceLoading={false}
			queuedMessageUnderEditID={null}
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
	describe.each([
		{
			viewer: "read-only viewer",
			isOwner: false,
			canUpdateChat: false,
			archived: false,
			canDelete: false,
			canPromote: false,
		},
		{
			viewer: "owner",
			isOwner: true,
			canUpdateChat: true,
			archived: false,
			canDelete: true,
			canPromote: true,
		},
		{
			viewer: "non-owner with update permission",
			isOwner: false,
			canUpdateChat: true,
			archived: false,
			canDelete: false,
			canPromote: false,
		},
		{
			viewer: "owner without update permission",
			isOwner: true,
			canUpdateChat: false,
			archived: false,
			canDelete: false,
			canPromote: false,
		},
		{
			viewer: "owner of an archived chat",
			isOwner: true,
			canUpdateChat: true,
			archived: true,
			canDelete: false,
			canPromote: false,
		},
	])(
		"queue permissions for $viewer",
		({ isOwner, canUpdateChat, archived, canDelete, canPromote }) => {
			it.each(["Remove from queue", "Send now", "Enter"])(
				"gates %s",
				async (action) => {
					const user = userEvent.setup();
					const onDeleteQueuedMessage = vi.fn();
					const onPromoteQueuedMessage = vi.fn();
					const mockChat = {
						...MockChat,
						id: "",
						owner_id: isOwner ? MockChat.owner_id : "other-user",
						archived,
					};
					const store = createChatStore();
					store.setQueuedMessages([MockChatQueuedMessage]);
					vi.spyOn(API, "checkAuthorization").mockImplementation(
						async ({ checks }) =>
							Object.fromEntries(
								Object.keys(checks).map((key) => [
									key,
									key === "canUpdateChat" ? canUpdateChat : true,
								]),
							),
					);
					const { queryClient } = renderChatPageInput(store, {
						chat: mockChat,
						onDeleteQueuedMessage,
						onPromoteQueuedMessage,
					});
					act(() => {
						queryClient.setQueryData(
							getAuthorizationKey({
								checks: {
									canUpdateChat: {
										object: {
											resource_type: "chat",
											owner_id: mockChat.owner_id,
											organization_id: mockChat.organization_id,
										},
										action: "update",
									},
								},
							}),
							{ canUpdateChat },
						);
					});

					if (action === "Enter") {
						await user.click(await screen.findByRole("textbox"));
						await user.keyboard("{Enter}");
					} else {
						await screen.findByRole("textbox");
						for (const button of screen.queryAllByRole("button", {
							name: action,
						})) {
							await user.click(button);
						}
					}

					expect(onDeleteQueuedMessage.mock.calls).toEqual(
						action === "Remove from queue" && canDelete
							? [[MockChatQueuedMessage.id]]
							: [],
					);
					expect(onPromoteQueuedMessage.mock.calls).toEqual(
						action !== "Remove from queue" && canPromote
							? [[MockChatQueuedMessage.id]]
							: [],
					);
				},
			);
		},
	);
	it.each([
		{ stage: "initial lookup", hasCachedPermissions: false },
		{ stage: "background refresh", hasCachedPermissions: true },
	])(
		"retries a failed $stage of queue permissions",
		async ({ hasCachedPermissions }) => {
			const user = userEvent.setup();
			const onDeleteQueuedMessage = vi.fn();
			const checkQueuePermissions = vi
				.fn()
				.mockResolvedValue({ canUpdateChat: true });
			if (hasCachedPermissions) {
				checkQueuePermissions.mockResolvedValueOnce({ canUpdateChat: true });
			}
			checkQueuePermissions.mockRejectedValueOnce(
				new Error("Unable to check queue permissions."),
			);
			vi.spyOn(API, "checkAuthorization").mockImplementation(
				async ({ checks }) =>
					"canUpdateChat" in checks
						? checkQueuePermissions()
						: Object.fromEntries(Object.keys(checks).map((key) => [key, true])),
			);
			const store = createChatStore();
			store.setQueuedMessages([MockChatQueuedMessage]);
			const { queryClient } = renderChatPageInput(store, {
				onDeleteQueuedMessage,
			});

			if (hasCachedPermissions) {
				await screen.findByRole("button", { name: "Remove from queue" });
				await act(async () => {
					await queryClient.refetchQueries({ queryKey: authorizationKey });
				});
			}

			await user.click(
				await screen.findByRole("button", { name: "Retry queue permissions" }),
			);
			await user.click(
				await screen.findByRole("button", { name: "Remove from queue" }),
			);

			expect(checkQueuePermissions).toHaveBeenCalledTimes(
				hasCachedPermissions ? 3 : 2,
			);
			expect(onDeleteQueuedMessage).toHaveBeenCalledWith(
				MockChatQueuedMessage.id,
			);
		},
	);

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

	it.each([
		{ status: "requires_action", interruptCalls: 1 },
		{ status: "interrupting", interruptCalls: 0 },
	] satisfies { status: TypesGen.ChatStatus; interruptCalls: number }[])(
		"routes Stop while the chat is $status",
		async ({ status, interruptCalls }) => {
			vi.spyOn(API, "getUserPreferenceSettings").mockResolvedValue(
				MockUserPreferenceSettings,
			);
			vi.spyOn(
				API.experimental,
				"getUserChatCompactionThresholds",
			).mockResolvedValue(MockUserChatCompactionThresholds);
			const user = userEvent.setup();
			const onInterrupt = vi.fn();
			const store = createChatStore();
			store.setChatStatus(status);

			renderChatPageInput(store, { onInterrupt });

			await user.click(await screen.findByRole("button", { name: "Stop" }));
			if (status === "interrupting") {
				await user.click(screen.getByRole("textbox", { name: "Chat message" }));
				await user.keyboard("{Escape}");
				expect(screen.getByRole("status")).toHaveTextContent(
					"Interrupting. Waiting for the agent to stop.",
				);
			}

			expect(onInterrupt).toHaveBeenCalledTimes(interruptCalls);
		},
	);

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
			editingTarget: { kind: "history", id: 1 },
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
