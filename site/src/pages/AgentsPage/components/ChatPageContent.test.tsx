import { act, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { createRef } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { API } from "#/api/api";
import { getAuthorizationKey } from "#/api/queries/authCheck";
import type * as TypesGen from "#/api/typesGenerated";
import { MockChat, MockChatQueuedMessage } from "#/testHelpers/chatEntities";
import { renderWithAuth } from "#/testHelpers/renderHelpers";
import { MockSkill } from "#/testHelpers/skills";
import type { ChatMessageInputRef } from "./AgentChatInput";
import { createChatStore } from "./ChatConversation/chatStore";
import { ChatPageInput } from "./ChatPageContent";

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
			canDelete: true,
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
			canDelete: true,
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
					const chat = {
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
						chat,
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
											owner_id: chat.owner_id,
											organization_id: chat.organization_id,
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
	it("retries a failed queue permission check before allowing removal", async () => {
		const user = userEvent.setup();
		const onDeleteQueuedMessage = vi.fn();
		const checkQueuePermissions = vi
			.fn()
			.mockRejectedValueOnce(new Error("Unable to check queue permissions."))
			.mockResolvedValue({ canUpdateChat: true });
		vi.spyOn(API, "checkAuthorization").mockImplementation(
			async ({ checks }) =>
				"canUpdateChat" in checks
					? checkQueuePermissions()
					: Object.fromEntries(Object.keys(checks).map((key) => [key, true])),
		);
		const store = createChatStore();
		store.setQueuedMessages([MockChatQueuedMessage]);
		renderChatPageInput(store, { onDeleteQueuedMessage });

		await user.click(
			await screen.findByRole("button", { name: "Retry queue permissions" }),
		);
		await user.click(
			await screen.findByRole("button", { name: "Remove from queue" }),
		);

		expect(checkQueuePermissions).toHaveBeenCalledTimes(2);
		expect(onDeleteQueuedMessage).toHaveBeenCalledWith(
			MockChatQueuedMessage.id,
		);
	});

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
