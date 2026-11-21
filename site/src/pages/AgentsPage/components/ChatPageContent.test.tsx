import { act, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { createRef, useState } from "react";
import {
	afterEach,
	beforeEach,
	describe,
	expect,
	it,
	type Mock,
	vi,
} from "vitest";
import { API } from "#/api/api";
import { authorizationKey, getAuthorizationKey } from "#/api/queries/authCheck";
import type * as TypesGen from "#/api/typesGenerated";
import { toast } from "#/components/Toaster/toast";
import {
	MockChat,
	MockChatQueuedMessage,
	MockMCPServerConfig,
} from "#/testHelpers/chatEntities";
import { createDeferred } from "#/testHelpers/deferred";
import {
	MockUserChatCompactionThresholds,
	MockUserPreferenceSettings,
	MockWorkspace,
	MockWorkspaceAgent,
} from "#/testHelpers/entities";
import { renderWithAuth } from "#/testHelpers/renderHelpers";
import { MockSkill } from "#/testHelpers/skills";
import { MessageScroller } from "#/vendor/message-scroller";
import type { ChatMessageInputRef } from "./AgentChatInput";
import { createChatStore } from "./ChatConversation/chatStore";
import {
	MockCollapsedStepsPreferences,
	MockWorkingMessages,
} from "./ChatConversation/storyFixtures";
import type { EditingTarget } from "./ChatConversation/types";
import {
	ChatPageInput,
	ChatPageTimeline,
	workspaceSkillsFromChat,
} from "./ChatPageContent";

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
) => renderWithAuth(chatPageInput(store, overrides));

// Renders the input with props a test can change while it stays mounted.
const renderUpdatableChatPageInput = (
	store: ReturnType<typeof createChatStore>,
	initialOverrides: Partial<React.ComponentProps<typeof ChatPageInput>>,
) => {
	let setOverrides: React.Dispatch<
		React.SetStateAction<Partial<React.ComponentProps<typeof ChatPageInput>>>
	> = () => {};
	const inputRef = { current: null };
	const Harness: React.FC = () => {
		const [overrides, setState] = useState(initialOverrides);
		setOverrides = setState;
		return chatPageInput(store, { inputRef, ...overrides });
	};
	renderWithAuth(<Harness />);
	return {
		update: (next: Partial<React.ComponentProps<typeof ChatPageInput>>) =>
			act(() => setOverrides((current) => ({ ...current, ...next }))),
	};
};

const chatPageInput = (
	store: ReturnType<typeof createChatStore>,
	overrides: Partial<React.ComponentProps<typeof ChatPageInput>>,
) => (
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
	/>
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
	// Every test uses the same chat ID, so a draft one test saves would
	// otherwise be restored in the next.
	localStorage.clear();
	// jsdom lacks Range geometry, which the slash menu reads to anchor.
	// The method does not exist to spy on, so add it per test and remove
	// it afterwards.
	Range.prototype.getBoundingClientRect = () => new DOMRect(0, 0, 1, 16);
});

afterEach(() => {
	vi.restoreAllMocks();
	Reflect.deleteProperty(Range.prototype, "getBoundingClientRect");
});

describe("ChatPageTimeline", () => {
	it("renders the chat record's status until the stream reports one", async () => {
		vi.spyOn(API, "getUserPreferenceSettings").mockResolvedValue(
			MockCollapsedStepsPreferences,
		);
		const store = createChatStore();
		store.seed({
			messages: MockWorkingMessages.slice(0, 5),
			queued_messages: [],
			has_more: false,
			snapshot_version: 1,
		});
		renderWithAuth(
			<MessageScroller.Provider autoScroll defaultScrollPosition="end">
				<ChatPageTimeline
					organizationId="organization-id"
					store={store}
					recordStatus="running"
					persistedError={undefined}
					hasMoreMessages={false}
					isFetchingMoreMessages={false}
					hasFetchMoreError={false}
					onFetchMoreMessages={vi.fn()}
					urlTransform={(url) => url}
					mcpServers={[]}
				/>
			</MessageScroller.Provider>,
		);
		expect(
			await screen.findByRole("button", { name: /^Working for/ }),
		).toBeInTheDocument();

		act(() => store.applyServerChatStatus("waiting"));
		expect(
			screen.getByRole("button", { name: /^Worked for/ }),
		).toBeInTheDocument();
	});
});

describe("ChatPageInput", () => {
	it("uses the chat record's status until the stream reports one", async () => {
		const store = createChatStore();
		renderChatPageInput(store, {
			chat: { ...MockChat, id: "", organization_id: "", status: "running" },
		});
		expect(
			await screen.findByRole("button", { name: "Stop" }),
		).toBeInTheDocument();

		act(() => store.applyServerChatStatus("waiting"));
		expect(screen.queryByRole("button", { name: "Stop" })).toBeNull();
	});

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

	it("routes Stop to onInterrupt while the chat requires action", async () => {
		const user = userEvent.setup();
		const onInterrupt = vi.fn();
		const store = createChatStore();
		store.applyServerChatStatus("requires_action");

		renderChatPageInput(store, { onInterrupt });

		await user.click(await screen.findByRole("button", { name: "Stop" }));
		expect(onInterrupt).toHaveBeenCalledTimes(1);
	});

	it("does not interrupt a read-only chat when Escape closes the MCP menu", async () => {
		const user = userEvent.setup();
		const onInterrupt = vi.fn();
		const store = createChatStore();
		store.applyServerChatStatus("running");

		renderChatPageInput(store, {
			isReadOnly: true,
			isInputDisabled: true,
			mcpServers: [MockMCPServerConfig],
			selectedMCPServerIds: [MockMCPServerConfig.id],
			onInterrupt,
		});

		await user.click(await screen.findByRole("button", { name: "1 MCP" }));
		await user.keyboard("{Escape}{Escape}");

		expect(onInterrupt).not.toHaveBeenCalled();
	});

	describe("workspace upload drafts", () => {
		const mockChat = {
			...MockChat,
			id: "workspace-upload-chat",
			organization_id: "",
			workspace_id: MockWorkspace.id,
		};
		const mockUploadedFile: TypesGen.UploadChatWorkspaceFileResponse = {
			path: `/home/coder/.coder/chats/${mockChat.id}/files/archive.tar`,
			name: "archive.tar",
			size: 16,
			media_type: "application/x-tar",
			workspace_id: MockWorkspace.id,
		};
		const draftProps = {
			chat: mockChat,
			workspace: MockWorkspace,
			workspaceAgent: MockWorkspaceAgent,
			initialValue: "Review this file",
		};
		const mockPendingUpload = {
			path: mockUploadedFile.path,
			name: mockUploadedFile.name,
			size: mockUploadedFile.size,
			mediaType: mockUploadedFile.media_type,
			workspaceId: mockUploadedFile.workspace_id,
		};

		beforeEach(() => {
			localStorage.clear();
			vi.spyOn(API, "getUserPreferenceSettings").mockResolvedValue(
				MockUserPreferenceSettings,
			);
			vi.spyOn(
				API.experimental,
				"getUserChatCompactionThresholds",
			).mockResolvedValue(MockUserChatCompactionThresholds);
			vi.spyOn(API.experimental, "getChatPrompts").mockResolvedValue({
				prompts: [],
			});
			vi.spyOn(API.experimental, "uploadChatWorkspaceFile").mockResolvedValue(
				mockUploadedFile,
			);
		});

		afterEach(() => localStorage.clear());

		it.each(["send", "remove"])(
			"restores completed uploads after a failed send and remount until %s",
			async (action) => {
				const user = userEvent.setup();
				const onSend = vi
					.fn()
					.mockRejectedValueOnce(new Error("Message could not be sent."))
					.mockResolvedValue(undefined);
				const props = { ...draftProps, onSend };
				const firstVisit = renderChatPageInput(createChatStore(), props);

				await user.upload(
					await screen.findByTestId("chat-attachment-file-input"),
					new File([new Uint8Array(16)], "archive.tar", {
						type: "application/x-tar",
					}),
				);
				await user.click(screen.getByRole("button", { name: "Send" }));
				await waitFor(() => expect(onSend).toHaveBeenCalledTimes(1));
				expect(onSend.mock.calls[0][0].workspaceUploads).toEqual([
					mockPendingUpload,
				]);
				firstVisit.unmount();

				const secondVisit = renderChatPageInput(createChatStore(), props);
				if (action === "remove") {
					await user.click(
						await screen.findByRole("button", { name: "Remove archive.tar" }),
					);
				} else {
					await user.click(await screen.findByRole("button", { name: "Send" }));
					await waitFor(() => expect(onSend).toHaveBeenCalledTimes(2));
					expect(onSend.mock.calls[1][0].workspaceUploads).toEqual([
						mockPendingUpload,
					]);
				}
				secondVisit.unmount();

				renderChatPageInput(createChatStore(), props);
				await user.click(await screen.findByRole("button", { name: "Send" }));
				await waitFor(() =>
					expect(onSend).toHaveBeenCalledTimes(action === "remove" ? 2 : 3),
				);
				expect(onSend).toHaveBeenLastCalledWith(
					expect.objectContaining({ workspaceUploads: undefined }),
				);
				expect(API.experimental.uploadChatWorkspaceFile).toHaveBeenCalledOnce();
			},
		);

		it("drops edit uploads when editing a different message", async () => {
			const user = userEvent.setup();
			const onSend = vi.fn().mockResolvedValue(undefined);
			const { update } = renderUpdatableChatPageInput(createChatStore(), {
				...draftProps,
				onSend,
				editingTarget: { kind: "history", id: 1 },
				editingFileBlocks: [],
			});

			await user.upload(
				await screen.findByTestId("chat-attachment-file-input"),
				new File([new Uint8Array(16)], "archive.tar", {
					type: "application/x-tar",
				}),
			);
			await screen.findByRole("button", { name: "Remove archive.tar" });
			update({
				editingTarget: { kind: "history", id: 2 },
				editingFileBlocks: [],
			});
			await user.click(
				await screen.findByRole("button", { name: "Save edit" }),
			);

			await waitFor(() => expect(onSend).toHaveBeenCalledTimes(1));
			expect(onSend.mock.calls[0][0].workspaceUploads).toBeUndefined();
		});

		it("keeps compose uploads out of message edits without clearing the draft", async () => {
			const user = userEvent.setup();
			const onSend = vi
				.fn()
				.mockRejectedValueOnce(new Error("Message could not be sent."))
				.mockResolvedValue(undefined);
			const props = { ...draftProps, onSend };
			const firstVisit = renderChatPageInput(createChatStore(), props);
			await user.upload(
				await screen.findByTestId("chat-attachment-file-input"),
				new File([new Uint8Array(16)], "archive.tar", {
					type: "application/x-tar",
				}),
			);
			await user.click(screen.getByRole("button", { name: "Send" }));
			await waitFor(() => expect(onSend).toHaveBeenCalledTimes(1));
			firstVisit.unmount();

			const editVisit = renderChatPageInput(createChatStore(), {
				...props,
				editingTarget: { kind: "history", id: 1 },
			});
			await user.click(
				await screen.findByRole("button", { name: "Save edit" }),
			);
			await waitFor(() => expect(onSend).toHaveBeenCalledTimes(2));
			expect(onSend.mock.calls[1][0].workspaceUploads).toBeUndefined();
			editVisit.unmount();

			renderChatPageInput(createChatStore(), props);
			await user.click(await screen.findByRole("button", { name: "Send" }));
			await waitFor(() => expect(onSend).toHaveBeenCalledTimes(3));
			expect(onSend.mock.calls[2][0].workspaceUploads).toEqual([
				mockPendingUpload,
			]);
		});

		it.each([
			{
				action: "add",
				name: "keeps saved files when saving an additional upload fails",
			},
			{
				action: "remove",
				name: "does not restore removed files when saving the remaining uploads fails",
			},
			{
				action: "remove",
				reusePath: true,
				sizeDelta: 1,
				name: "does not restore a removed upload when another upload reused its path",
			},
			{
				action: "remove",
				reusePath: true,
				name: "does not restore duplicate uploads after a failed removal",
			},
			{
				action: "remove-unsaved",
				name: "keeps saved files after unsaved removal",
			},
			{
				action: "remove-pending",
				name: "keeps saved files after pending removal",
			},
		])("$name", async ({ action, reusePath, sizeDelta = 0 }) => {
			const user = userEvent.setup();
			const onSend = vi
				.fn()
				.mockRejectedValueOnce(new Error("Message could not be sent."))
				.mockResolvedValue(undefined);
			const warning = vi.spyOn(toast, "warning");
			const mockRemainingFile = {
				...mockUploadedFile,
				path: reusePath
					? mockUploadedFile.path
					: `/home/coder/.coder/chats/${mockChat.id}/files/keep.tar`,
				name: reusePath ? mockUploadedFile.name : "keep.tar",
				size: mockUploadedFile.size + sizeDelta,
			};
			const uploadFile = vi.mocked(API.experimental.uploadChatWorkspaceFile);
			uploadFile.mockResolvedValueOnce(mockUploadedFile);
			if (action === "remove-pending") {
				uploadFile.mockImplementationOnce(
					() => new Promise<TypesGen.UploadChatWorkspaceFileResponse>(() => {}),
				);
			} else {
				uploadFile.mockResolvedValueOnce(mockRemainingFile);
			}
			const mockPendingRemainingUpload = {
				...mockPendingUpload,
				path: mockRemainingFile.path,
				name: mockRemainingFile.name,
				size: mockRemainingFile.size,
			};
			const props = { ...draftProps, onSend };
			const firstVisit = renderChatPageInput(createChatStore(), props);
			const savedFiles =
				action === "remove"
					? [mockUploadedFile, mockRemainingFile]
					: [mockUploadedFile];
			await user.upload(
				await screen.findByTestId("chat-attachment-file-input"),
				savedFiles.map(
					(file) =>
						new File([new Uint8Array(file.size)], file.name, {
							type: file.media_type,
						}),
				),
			);
			await waitFor(() =>
				expect(
					JSON.parse(
						localStorage.getItem(
							`agents.workspace-upload-draft.${mockChat.id}.${mockChat.workspace_id}`,
						) ?? "[]",
					),
				).toEqual(savedFiles),
			);

			vi.spyOn(Storage.prototype, "setItem").mockImplementation(() => {
				throw new DOMException("Storage is full.", "QuotaExceededError");
			});
			if (action === "remove") {
				await user.click(
					screen.getAllByRole("button", { name: "Remove archive.tar" })[0],
				);
			} else {
				await user.upload(
					screen.getByTestId("chat-attachment-file-input"),
					new File([new Uint8Array(16)], "keep.tar", {
						type: "application/x-tar",
					}),
				);
				if (action === "remove-unsaved") {
					await waitFor(() => expect(warning).toHaveBeenCalledOnce());
				}
				if (action !== "add") {
					await user.click(
						screen.getByRole("button", { name: "Remove keep.tar" }),
					);
				}
			}
			if (action !== "remove-pending") {
				await waitFor(() => expect(warning).toHaveBeenCalled());
			}
			await user.click(screen.getByRole("button", { name: "Send" }));
			await waitFor(() => expect(onSend).toHaveBeenCalledTimes(1));
			expect(onSend.mock.calls[0][0].workspaceUploads).toEqual(
				action === "add"
					? [mockPendingUpload, mockPendingRemainingUpload]
					: action === "remove"
						? [mockPendingRemainingUpload]
						: [mockPendingUpload],
			);
			firstVisit.unmount();

			renderChatPageInput(createChatStore(), props);
			await user.click(await screen.findByRole("button", { name: "Send" }));
			await waitFor(() => expect(onSend).toHaveBeenCalledTimes(2));
			expect(onSend.mock.calls[1][0].workspaceUploads).toEqual(
				action === "remove" ? undefined : [mockPendingUpload],
			);
		});
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
			editingTarget: { kind: "history", id: 1 },
			initialValue: "edited",
			editingFileBlocks: [
				workspaceFileReference("current.csv", "ws-1"),
				workspaceFileReference("other.csv", "ws-2"),
				workspaceFileReference("unbound.csv", ""),
			],
		});

		await user.click(await screen.findByRole("button", { name: "Save edit" }));

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

	describe("edited workspace file references", () => {
		type OnSendMock = Mock<
			React.ComponentProps<typeof ChatPageInput>["onSend"]
		>;
		const editing: EditingTarget = { kind: "history", id: 1 };
		const mockBoundChat = {
			...MockChat,
			id: "",
			organization_id: "",
			workspace_id: "ws-1",
		};
		const editingFileBlocks = [
			workspaceFileReference("first.csv", "ws-1"),
			workspaceFileReference("second.csv", "ws-1"),
			workspaceFileReference("rebound.csv", "ws-2"),
		];
		const sentFileNames = (onSend: OnSendMock, call: number) =>
			onSend.mock.calls[call][0].workspaceUploads?.map((upload) => upload.name);
		const renderEdit = (onSend: OnSendMock) =>
			renderUpdatableChatPageInput(createChatStore(), {
				chat: mockBoundChat,
				onSend,
				editingTarget: editing,
				initialValue: "edited",
				editingFileBlocks,
			});
		const saveEdit = async (
			user: ReturnType<typeof userEvent.setup>,
			onSend: OnSendMock,
			calls: number,
		) => {
			await user.click(
				await screen.findByRole("button", { name: "Save edit" }),
			);
			await waitFor(() => expect(onSend).toHaveBeenCalledTimes(calls));
		};

		it("keeps a removed reference removed after a failed edit", async () => {
			const user = userEvent.setup();
			const firstSend = createDeferred<undefined>();
			const onSend = vi
				.fn()
				.mockReturnValueOnce(firstSend.promise)
				.mockResolvedValue(undefined);
			const { update } = renderEdit(onSend);

			await user.click(
				await screen.findByRole("button", { name: "Remove first.csv" }),
			);
			await saveEdit(user, onSend, 1);
			update({ editingTarget: null, isSendPending: true });
			await act(async () => firstSend.reject(new Error("Edit failed.")));
			update({ editingTarget: editing, isSendPending: false });
			await saveEdit(user, onSend, 2);

			expect(sentFileNames(onSend, 1)).toEqual(["second.csv"]);
		});

		it("drops references when the workspace is rebound away and back during a pending edit", async () => {
			const user = userEvent.setup();
			const firstSend = createDeferred<undefined>();
			const onSend = vi
				.fn()
				.mockReturnValueOnce(firstSend.promise)
				.mockResolvedValue(undefined);
			const { update } = renderEdit(onSend);

			await saveEdit(user, onSend, 1);
			update({ editingTarget: null, isSendPending: true });
			update({ chat: { ...mockBoundChat, workspace_id: "ws-2" } });
			update({ chat: mockBoundChat });
			await act(async () => firstSend.reject(new Error("Edit failed.")));
			update({ editingTarget: editing, isSendPending: false });
			await saveEdit(user, onSend, 2);

			expect(sentFileNames(onSend, 1)).toBeUndefined();
		});

		it("rehydrates references for a workspace rebound mid-edit", async () => {
			const user = userEvent.setup();
			const onSend = vi.fn().mockResolvedValue(undefined);
			const { update } = renderEdit(onSend);

			await screen.findByRole("button", { name: "Remove first.csv" });
			update({ chat: { ...mockBoundChat, workspace_id: "ws-2" } });
			await saveEdit(user, onSend, 1);

			expect(sentFileNames(onSend, 0)).toEqual(["rebound.csv"]);
		});

		it("drops references when the chat changes mid-edit", async () => {
			const user = userEvent.setup();
			const onSend = vi.fn().mockResolvedValue(undefined);
			const { update } = renderEdit(onSend);

			await screen.findByRole("button", { name: "Remove first.csv" });
			update({ chat: { ...mockBoundChat, id: "other-chat" } });
			await saveEdit(user, onSend, 1);

			expect(sentFileNames(onSend, 0)).toBeUndefined();
		});

		it("drops references after a successful edit", async () => {
			const user = userEvent.setup();
			const onSend = vi.fn().mockResolvedValue(undefined);
			renderEdit(onSend);

			await saveEdit(user, onSend, 1);
			await saveEdit(user, onSend, 2);

			expect(sentFileNames(onSend, 0)).toEqual(["first.csv", "second.csv"]);
			expect(sentFileNames(onSend, 1)).toBeUndefined();
		});

		it("restores removed references when the edit is reopened", async () => {
			const user = userEvent.setup();
			const onSend = vi.fn().mockResolvedValue(undefined);
			const { update } = renderEdit(onSend);

			await user.click(
				await screen.findByRole("button", { name: "Remove first.csv" }),
			);
			update({ editingTarget: null });
			update({ editingTarget: editing });
			await saveEdit(user, onSend, 1);

			expect(sentFileNames(onSend, 0)).toEqual(["first.csv", "second.csv"]);
		});
	});

	it("sends the attachments of a failed send again", async () => {
		const user = userEvent.setup();
		vi.spyOn(API.experimental, "uploadChatFile").mockResolvedValue({
			id: "file-1",
		});
		const firstSend = createDeferred<undefined>();
		const onSend = vi
			.fn()
			.mockReturnValueOnce(firstSend.promise)
			.mockResolvedValue(undefined);

		renderChatPageInput(createChatStore(), { onSend });
		await user.upload(
			await screen.findByTestId("chat-attachment-file-input"),
			new File(["png"], "shot.png", { type: "image/png" }),
		);
		const send = screen.getByRole("button", { name: "Send" });
		await waitFor(() => expect(send).toBeEnabled());
		await user.click(send);

		const sent = { fileId: "file-1", mediaType: "image/png" };
		expect(onSend).toHaveBeenLastCalledWith(
			expect.objectContaining({ attachments: [sent] }),
		);

		firstSend.reject(new Error("send failed"));
		await screen.findByRole("button", { name: "Remove shot.png" });
		await user.click(send);
		expect(onSend).toHaveBeenCalledTimes(2);
		expect(onSend).toHaveBeenLastCalledWith(
			expect.objectContaining({ attachments: [sent] }),
		);
	});

	it("keeps an attachment added while a send is pending", async () => {
		const user = userEvent.setup();
		vi.spyOn(API.experimental, "uploadChatFile")
			.mockResolvedValueOnce({ id: "file-1" })
			.mockResolvedValueOnce({ id: "file-2" });
		const firstSend = createDeferred<undefined>();
		const onSend = vi
			.fn()
			.mockReturnValueOnce(firstSend.promise)
			.mockResolvedValue(undefined);

		renderChatPageInput(createChatStore(), { onSend });
		const fileInput = await screen.findByTestId("chat-attachment-file-input");
		await user.upload(
			fileInput,
			new File(["png"], "first.png", { type: "image/png" }),
		);
		const send = await screen.findByRole("button", { name: "Send" });
		await waitFor(() => expect(send).toBeEnabled());
		await user.click(send);
		await user.upload(
			fileInput,
			new File(["png"], "second.png", { type: "image/png" }),
		);
		await screen.findByRole("button", { name: "Remove second.png" });

		await act(async () => {
			firstSend.resolve(undefined);
		});
		await waitFor(() => expect(send).toBeEnabled());
		await user.click(send);

		expect(onSend).toHaveBeenLastCalledWith(
			expect.objectContaining({
				attachments: [{ fileId: "file-2", mediaType: "image/png" }],
			}),
		);
	});

	it("offers no queue actions on a row a pending send shows", async () => {
		const user = userEvent.setup();
		const onPromoteQueuedMessage = vi.fn();
		const onDeleteQueuedMessage = vi.fn();
		vi.spyOn(API, "checkAuthorization").mockImplementation(async ({ checks }) =>
			Object.fromEntries(Object.keys(checks).map((key) => [key, true])),
		);
		const store = createChatStore();
		store.seed({
			messages: [],
			queued_messages: [],
			has_more: false,
			snapshot_version: 1,
		});
		store.applyServerChatStatus("running");
		void store.runPendingAction(
			{
				kind: "send",
				busyBehavior: "queue",
				chatID: "",
				content: [{ type: "text", text: "not stored yet" }],
			},
			() => new Promise(() => {}),
		);

		renderChatPageInput(store, {
			onPromoteQueuedMessage,
			onDeleteQueuedMessage,
		});
		await screen.findByText("not stored yet");
		await user.click(await screen.findByRole("button", { name: "Send now" }));
		await user.click(screen.getByRole("button", { name: "Remove from queue" }));
		await user.click(screen.getByRole("textbox", { name: "Chat message" }));
		await user.keyboard("{Enter}");

		expect(onPromoteQueuedMessage).not.toHaveBeenCalled();
		expect(onDeleteQueuedMessage).not.toHaveBeenCalled();
	});
});
