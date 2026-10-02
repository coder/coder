import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http } from "msw";
import { describe, expect, it, vi } from "vitest";
import { API } from "#/api/api";
import type * as ChatAutomationQueries from "#/api/queries/chatAutomations";
import { chatAutomationNameMap } from "#/api/queries/chatAutomations";
import type * as TypesGen from "#/api/typesGenerated";
import {
	MockChat,
	MockChatAutomation,
	MockChatMessage,
	MockChatQueuedMessage,
} from "#/testHelpers/chatEntities";
import { renderWithAuth } from "#/testHelpers/renderHelpers";
import { server } from "#/testHelpers/server";
import { MessageScroller } from "#/vendor/message-scroller";
import { createChatStore } from "./ChatConversation/chatStore";
import {
	ChatPageInput,
	ChatPageTimeline,
	workspaceSkillsFromChat,
} from "./ChatPageContent";

// Wraps the real query options so tests without a rendered outcome can wait
// until the component has rendered. The dashboard renders children only after
// it loads the experiments, so by then any automations request has started.
vi.mock("#/api/queries/chatAutomations", async (importOriginal) => {
	const actual = await importOriginal<typeof ChatAutomationQueries>();
	return {
		...actual,
		chatAutomationNameMap: vi.fn(actual.chatAutomationNameMap),
	};
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
			editingTarget={null}
			onCancelEdit={vi.fn()}
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

const mockQueuedAutomationInput: TypesGen.ChatQueuedMessage = {
	...MockChatQueuedMessage,
	automation_id: MockChatAutomation.id,
	input_id: "0b6c4e2a-1f3d-4b5c-8a9e-7d6c5b4a3f2e",
};

const mockExperiments = (experiments: TypesGen.Experiment[]) =>
	server.use(
		http.get("/api/v2/experiments", () => HttpResponse.json(experiments)),
	);

const mockChatAutomationsResponse = () => {
	mockExperiments(["chat-automations"]);
	server.use(
		http.get(
			"/api/experimental/organizations/:organizationId/chat-automations",
			() => HttpResponse.json([MockChatAutomation]),
		),
	);
};

describe("ChatPageInput", () => {
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

	it("requests the automations list for the chat's organization when the queue has automation input", async () => {
		mockChatAutomationsResponse();
		const getChatAutomations = vi.spyOn(API.experimental, "getChatAutomations");
		const store = createChatStore();
		store.setQueuedMessages([mockQueuedAutomationInput]);

		renderChatPageInput(store, {
			chat: { ...MockChat, id: "", organization_id: "test-org-id" },
		});

		await waitFor(() =>
			expect(getChatAutomations).toHaveBeenCalledWith("test-org-id"),
		);
	});

	it.each<{
		name: string;
		experiments: TypesGen.Experiment[];
		automated: boolean;
	}>([
		{
			name: "the chat has no automation input",
			experiments: ["chat-automations"],
			automated: false,
		},
		{
			name: "the chat-automations experiment is off",
			experiments: [],
			automated: true,
		},
	])(
		"does not request automations when $name",
		async ({ experiments, automated }) => {
			mockExperiments(experiments);
			const getChatAutomations = vi.spyOn(
				API.experimental,
				"getChatAutomations",
			);
			const store = createChatStore();
			store.setQueuedMessages([
				automated ? mockQueuedAutomationInput : MockChatQueuedMessage,
			]);

			renderChatPageInput(store, {
				chat: { ...MockChat, id: "", organization_id: "test-org-id" },
			});

			await waitFor(() => expect(chatAutomationNameMap).toHaveBeenCalled());
			expect(getChatAutomations).not.toHaveBeenCalled();
		},
	);
});

const automationInputID = "0b6c4e2a-1f3d-4b5c-8a9e-7d6c5b4a3f2e";

const renderChatPageTimelineWithAutomationInput = () => {
	const store = createChatStore();
	store.replaceMessages([
		{
			...MockChatMessage,
			id: 1,
			automation_id: MockChatAutomation.id,
			input_id: automationInputID,
		},
		{ ...MockChatMessage, id: 2 },
	]);

	return renderWithAuth(
		<MessageScroller.Provider autoScroll defaultScrollPosition="end">
			<ChatPageTimeline
				organizationId="test-org-id"
				store={store}
				persistedError={undefined}
				hasMoreMessages={false}
				isFetchingMoreMessages={false}
				isHydratingMessages={false}
				hasFetchMoreError={false}
				onFetchMoreMessages={async () => {}}
			/>
		</MessageScroller.Provider>,
	);
};

describe("ChatPageTimeline", () => {
	it("requests the automations list for the chat's organization when history has automation input", async () => {
		mockChatAutomationsResponse();
		const getChatAutomations = vi.spyOn(API.experimental, "getChatAutomations");

		renderChatPageTimelineWithAutomationInput();

		await waitFor(() =>
			expect(getChatAutomations).toHaveBeenCalledWith("test-org-id"),
		);
	});

	it("does not request the automations list when the chat-automations experiment is off", async () => {
		mockExperiments([]);
		const getChatAutomations = vi.spyOn(API.experimental, "getChatAutomations");

		renderChatPageTimelineWithAutomationInput();

		await waitFor(() => expect(chatAutomationNameMap).toHaveBeenCalled());
		expect(getChatAutomations).not.toHaveBeenCalled();
	});
});
