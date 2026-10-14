import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { API } from "#/api/api";
import type * as TypesGen from "#/api/typesGenerated";
import { MockChat } from "#/testHelpers/chatEntities";
import { renderWithAuth } from "#/testHelpers/renderHelpers";
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

describe("ChatPageInput", () => {
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

	it("holds the attachments of a pending send and restores them when it fails", async () => {
		const user = userEvent.setup();
		vi.spyOn(API.experimental, "uploadChatFile").mockResolvedValue({
			id: "file-1",
		});
		let rejectSend: (error: Error) => void = () => {};
		const onSend = vi
			.fn()
			.mockReturnValueOnce(
				new Promise((_resolve, reject) => {
					rejectSend = reject;
				}),
			)
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

		rejectSend(new Error("send failed"));
		await screen.findByRole("button", { name: "Remove shot.png" });
		await user.click(send);
		expect(onSend).toHaveBeenCalledTimes(2);
		expect(onSend).toHaveBeenLastCalledWith(
			expect.objectContaining({ attachments: [sent] }),
		);
	});

	it("offers no queue actions on a row a pending send draws", async () => {
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
		store.addPendingAction({
			kind: "send",
			message: {
				id: 0,
				chat_id: "",
				created_at: "2025-01-01T00:00:00.000Z",
				role: "user",
				content: [{ type: "text", text: "not stored yet" }],
			},
		});

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
