import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { createRef } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { API } from "#/api/api";
import type * as TypesGen from "#/api/typesGenerated";
import { MockChat } from "#/testHelpers/chatEntities";
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
