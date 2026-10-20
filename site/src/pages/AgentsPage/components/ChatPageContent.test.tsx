import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { API } from "#/api/api";
import type * as TypesGen from "#/api/typesGenerated";
import { MockChat } from "#/testHelpers/chatEntities";
import { MockWorkspace, MockWorkspaceAgent } from "#/testHelpers/entities";
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
	afterEach(() => {
		vi.restoreAllMocks();
		localStorage.clear();
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

	it("keeps edited workspace attachments as workspace files", async () => {
		const user = userEvent.setup();
		const onSend = vi.fn();

		renderChatPageInput(createChatStore(), {
			onSend,
			isEditing: true,
			initialValue: "edited",
			editingFileBlocks: [
				{
					type: "workspace-attachment",
					file_id: "delivered-zip",
					name: "bundle.zip",
					media_type: "application/zip",
					size: 2,
				},
			],
		});

		await user.click(await screen.findByRole("button", { name: "Save Edit" }));

		await waitFor(() => expect(onSend).toHaveBeenCalledTimes(1));
		expect(onSend.mock.calls[0][0].attachments).toEqual([
			{
				fileId: "delivered-zip",
				mediaType: "application/zip",
				purpose: "workspace",
			},
		]);
	});

	it("stages workspace files when the chat has no server-picked agent", async () => {
		const user = userEvent.setup();
		const onSend = vi.fn();
		const uploadChatFile = vi
			.spyOn(API.experimental, "uploadChatFile")
			.mockResolvedValue({ id: "staged-zip" });
		const uploadChatWorkspaceFile = vi.spyOn(
			API.experimental,
			"uploadChatWorkspaceFile",
		);
		const zip = new File(["PK"], "bundle.zip", { type: "application/zip" });

		renderChatPageInput(createChatStore(), {
			chat: {
				...MockChat,
				id: "chat-staging",
				workspace_id: MockWorkspace.id,
			},
			workspace: MockWorkspace,
			onSend,
		});
		await user.upload(
			await screen.findByTestId("chat-attachment-file-input"),
			zip,
		);
		await waitFor(() =>
			expect(uploadChatFile).toHaveBeenCalledWith(
				zip,
				MockChat.organization_id,
				"workspace",
			),
		);
		await user.click(await screen.findByRole("button", { name: "Send" }));

		await waitFor(() => expect(onSend).toHaveBeenCalledTimes(1));
		expect(onSend.mock.calls[0][0].attachments).toEqual([
			{
				fileId: "staged-zip",
				mediaType: "application/zip",
				purpose: "workspace",
			},
		]);
		expect(uploadChatWorkspaceFile).not.toHaveBeenCalled();
	});

	it("streams workspace files through the chat's connected agent", async () => {
		const user = userEvent.setup();
		const uploadChatFile = vi.spyOn(API.experimental, "uploadChatFile");
		const uploadChatWorkspaceFile = vi
			.spyOn(API.experimental, "uploadChatWorkspaceFile")
			.mockResolvedValue({
				path: "/home/coder/.coder/chats/chat-streaming/files/bundle.zip",
				name: "bundle.zip",
				size: 2,
				media_type: "application/zip",
				workspace_id: MockWorkspace.id,
			});
		const zip = new File(["PK"], "bundle.zip", { type: "application/zip" });

		renderChatPageInput(createChatStore(), {
			chat: {
				...MockChat,
				id: "chat-streaming",
				workspace_id: MockWorkspace.id,
				agent_id: MockWorkspaceAgent.id,
			},
			workspace: MockWorkspace,
			workspaceAgent: MockWorkspaceAgent,
		});
		await user.upload(
			await screen.findByTestId("chat-attachment-file-input"),
			zip,
		);

		await waitFor(() =>
			expect(uploadChatWorkspaceFile).toHaveBeenCalledWith(
				"chat-streaming",
				zip,
				expect.anything(),
			),
		);
		expect(uploadChatFile).not.toHaveBeenCalled();
	});
});
