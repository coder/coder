import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http } from "msw";
import { describe, expect, it, vi } from "vitest";
import { API } from "#/api/api";
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

const queuedAutomationInput = (
	id: number,
	automationId: string,
	inputId: string,
): TypesGen.ChatQueuedMessage => ({
	...MockChatQueuedMessage,
	id,
	automation_id: automationId,
	input_id: inputId,
});

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

	it("requests the automations list for the chat's organization when the queue has automation input", async () => {
		mockChatAutomationsResponse();
		const getChatAutomations = vi.spyOn(API.experimental, "getChatAutomations");
		const store = createChatStore();
		store.setQueuedMessages([
			queuedAutomationInput(
				1,
				MockChatAutomation.id,
				"0b6c4e2a-1f3d-4b5c-8a9e-7d6c5b4a3f2e",
			),
		]);

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
				automated
					? queuedAutomationInput(
							1,
							MockChatAutomation.id,
							"0b6c4e2a-1f3d-4b5c-8a9e-7d6c5b4a3f2e",
						)
					: MockChatQueuedMessage,
			]);

			renderChatPageInput(store, {
				chat: { ...MockChat, id: "", organization_id: "test-org-id" },
			});

			await screen.findByRole("button", { name: "Send now" });
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

	it("shows the automation ID without requesting the list when the chat-automations experiment is off", async () => {
		mockExperiments([]);
		const getChatAutomations = vi.spyOn(API.experimental, "getChatAutomations");

		renderChatPageTimelineWithAutomationInput();

		await screen.findByRole("button", {
			name: "Automation run · 7f1c2b9e-4d3a-4c1f-9b2e-5a6d7e8f9a0b · input 0b6c4e2a",
		});
		expect(getChatAutomations).not.toHaveBeenCalled();
	});
});
