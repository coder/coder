import { screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import type * as TypesGen from "#/api/typesGenerated";
import { MockChat } from "#/testHelpers/chatEntities";
import { renderWithAuth } from "#/testHelpers/renderHelpers";
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
	onInterrupt: () => void,
) =>
	renderWithAuth(
		<ChatPageInput
			chat={{ ...MockChat, id: "", organization_id: "" }}
			store={store}
			models={[]}
			onSend={vi.fn()}
			onDeleteQueuedMessage={vi.fn()}
			onPromoteQueuedMessage={vi.fn()}
			onInterrupt={onInterrupt}
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
		/>,
	);

describe("ChatPageInput", () => {
	it("routes Stop to onInterrupt while the chat requires action", async () => {
		const user = userEvent.setup();
		const onInterrupt = vi.fn();
		const store = createChatStore();
		store.setChatStatus("requires_action");

		renderChatPageInput(store, onInterrupt);

		await user.click(await screen.findByRole("button", { name: "Stop" }));
		expect(onInterrupt).toHaveBeenCalledTimes(1);
	});
});
