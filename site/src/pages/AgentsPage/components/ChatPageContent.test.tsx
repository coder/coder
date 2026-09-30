import { act, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http } from "msw";
import { describe, expect, it, vi } from "vitest";
import { API } from "#/api/api";
import type * as TypesGen from "#/api/typesGenerated";
import {
	MockChat,
	MockChatAutomation,
	MockChatQueuedMessage,
} from "#/testHelpers/chatEntities";
import { renderWithAuth } from "#/testHelpers/renderHelpers";
import { server } from "#/testHelpers/server";
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

const deletedAutomationId = "3e9d8c7b-6a5f-4e3d-8c2b-1a0f9e8d7c6b";

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

const useExperiments = (experiments: TypesGen.Experiment[]) =>
	server.use(
		http.get("/api/v2/experiments", () => HttpResponse.json(experiments)),
	);

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

	it("labels queued automation input by automation name, or by ID once the automation is deleted", async () => {
		useExperiments(["chat-automations"]);
		server.use(
			http.get(
				"/api/experimental/organizations/:organizationId/chat-automations",
				() => HttpResponse.json([MockChatAutomation]),
			),
		);
		const store = createChatStore();
		store.setQueuedMessages([
			queuedAutomationInput(
				1,
				MockChatAutomation.id,
				"0b6c4e2a-1f3d-4b5c-8a9e-7d6c5b4a3f2e",
			),
			queuedAutomationInput(
				2,
				deletedAutomationId,
				"9a8b7c6d-5e4f-4a3b-9c2d-1e0f2a3b4c5d",
			),
			{ ...MockChatQueuedMessage, id: 3 },
		]);

		renderChatPageInput(store, {
			chat: { ...MockChat, id: "", organization_id: "test-org-id" },
		});

		await screen.findByRole("button", {
			name: "Automation run · CI heartbeat · input 0b6c4e2a",
		});
		const labels = screen.getAllByRole("button", { name: /^Automation run/ });
		expect(labels.map((label) => label.textContent)).toEqual([
			"Automation run · CI heartbeat · input 0b6c4e2a",
			`Automation run · ${deletedAutomationId} · input 9a8b7c6d`,
		]);

		// A truncated name stays readable in the tooltip, and the ID
		// fallback explains why the name is missing. Keyboard focus opens
		// each tooltip in turn; jsdom hover leaves Radix's pointer grace
		// area engaged and keeps the second tooltip closed.
		act(() => labels[0].focus());
		expect(await screen.findByRole("tooltip")).toHaveTextContent(
			"Automation: CI heartbeat",
		);
		act(() => labels[1].focus());
		await waitFor(() =>
			expect(screen.getByRole("tooltip")).toHaveTextContent(
				"Automation name unavailable",
			),
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
			useExperiments(experiments);
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
