import { act, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { createRef } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { API } from "#/api/api";
import type * as TypesGen from "#/api/typesGenerated";
import { MockChat } from "#/testHelpers/chatEntities";
import { createDeferred } from "#/testHelpers/deferred";
import { renderWithAuth } from "#/testHelpers/renderHelpers";
import { MockSkill } from "#/testHelpers/skills";
import type { ChatMessageInputRef } from "./AgentChatInput";
import { createChatStore } from "./ChatConversation/chatStore";
import { ChatPageInput, workspaceSkillsFromChat } from "./ChatPageContent";

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
		store.applyServerChatStatus("requires_action");

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

	it("holds the attachments of a pending send and restores them when it fails", async () => {
		const user = userEvent.setup();
		vi.spyOn(API.experimental, "uploadChatFile").mockResolvedValue({
			id: "file-1",
		});
		const firstSend = createDeferred<void>();
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
		expect(
			screen.queryByRole("button", { name: "Remove shot.png" }),
		).toBeNull();

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
		const firstSend = createDeferred<void>();
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
			firstSend.resolve();
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
		const store = createChatStore();
		store.seed({
			messages: [],
			queued_messages: [],
			has_more: false,
			snapshot_version: 1,
		});
		store.applyServerChatStatus("running");
		store.addPendingAction(
			{
				kind: "send",
				message: {
					id: 0,
					chat_id: "",
					created_at: "2025-01-01T00:00:00.000Z",
					role: "user",
					content: [{ type: "text", text: "not stored yet" }],
				},
			},
			"running",
		);

		renderChatPageInput(store, {
			onPromoteQueuedMessage,
			onDeleteQueuedMessage,
		});
		await screen.findByText("not stored yet");
		await user.click(screen.getByRole("button", { name: "Send now" }));
		await user.click(screen.getByRole("button", { name: "Remove from queue" }));
		await user.click(screen.getByTestId("chat-message-input"));
		await user.keyboard("{Enter}");

		expect(onPromoteQueuedMessage).not.toHaveBeenCalled();
		expect(onDeleteQueuedMessage).not.toHaveBeenCalled();
	});

	it("lets another queued row act while one row's request is in flight", async () => {
		const user = userEvent.setup();
		const onDeleteQueuedMessage = vi
			.fn()
			.mockReturnValueOnce(new Promise(() => {}))
			.mockResolvedValue(undefined);
		const queuedRow = (id: number, text: string) => ({
			id,
			chat_id: "",
			created_at: "2025-01-01T00:00:00.000Z",
			content: [{ type: "text" as const, text }],
		});
		const store = createChatStore();
		store.seed({
			messages: [],
			queued_messages: [
				queuedRow(1, "first queued"),
				queuedRow(2, "second queued"),
			],
			has_more: false,
			snapshot_version: 1,
		});
		store.applyServerChatStatus("running");

		renderChatPageInput(store, { onDeleteQueuedMessage });
		await screen.findByText("second queued");
		const [removeFirst, removeSecond] = screen.getAllByRole("button", {
			name: "Remove from queue",
		});
		await user.click(removeFirst);

		expect(removeFirst).toBeDisabled();
		expect(removeSecond).toBeEnabled();
		await user.click(removeSecond);
		expect(onDeleteQueuedMessage.mock.calls).toEqual([[1], [2]]);
	});
});
