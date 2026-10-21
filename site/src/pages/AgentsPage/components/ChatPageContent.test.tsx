import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useRef, useState } from "react";
import { useMutation } from "react-query";
import { afterEach, beforeAll, describe, expect, it, vi } from "vitest";
import { API } from "#/api/api";
import type * as TypesGen from "#/api/typesGenerated";
import {
	MockChat,
	MockChatWorkspaceAttachmentPart,
} from "#/testHelpers/chatEntities";
import { MockWorkspace, MockWorkspaceAgent } from "#/testHelpers/entities";
import { renderWithAuth } from "#/testHelpers/renderHelpers";
import {
	type SendChatTurnOptions,
	useConversationEditingState,
} from "../hooks/useConversationEditingState";
import type { ChatMessageInputRef } from "./AgentChatInput";
import { createChatStore } from "./ChatConversation/chatStore";
import { ChatPageInput } from "./ChatPageContent";

type ChatPageInputProps = React.ComponentProps<typeof ChatPageInput>;

const defaultChatPageInputProps = (
	store: ReturnType<typeof createChatStore>,
): ChatPageInputProps => ({
	chat: { ...MockChat, id: "", organization_id: "" },
	store,
	onSend: vi.fn(),
	onDeleteQueuedMessage: vi.fn(),
	onPromoteQueuedMessage: vi.fn(),
	onInterrupt: vi.fn(),
	isInputDisabled: false,
	isSendPending: false,
	isInterruptPending: false,
	hasModelOptions: true,
	selectedModel: "model-config-1",
	onModelChange: vi.fn(),
	modelOptions: [
		{
			id: "model-config-1",
			provider: "openai",
			model: "gpt-4o",
			displayName: "GPT-4o",
		},
	],
	modelSelectorPlaceholder: "Select model",
	canConfigureAgentSetup: false,
	isEditing: false,
	onCancelHistoryEdit: vi.fn(),
	isReadOnly: false,
	onReasoningEffortChange: vi.fn(),
	unsupportedProviderNames: [],
	onPlanModeToggle: vi.fn(),
	isModelCatalogLoading: false,
	inputRef: { current: null },
	initialValue: "",
	remountKey: 0,
	onContentChange: vi.fn(),
	editingFileBlocks: [],
	mcpServers: [],
	selectedMCPServerIds: [],
	onMCPSelectionChange: vi.fn(),
	onMCPAuthComplete: vi.fn(),
	isWorkspaceLoading: false,
});

const renderChatPageInput = (
	store: ReturnType<typeof createChatStore>,
	overrides: Partial<ChatPageInputProps> = {},
) =>
	renderWithAuth(
		<ChatPageInput {...defaultChatPageInputProps(store)} {...overrides} />,
	);

type HistoryEditHarnessProps = {
	fileBlocks: readonly TypesGen.ChatMessagePart[];
	sendTurn: (options: SendChatTurnOptions) => Promise<void>;
	overrides: Partial<ChatPageInputProps>;
};

const HistoryEditHarness: React.FC<HistoryEditHarnessProps> = ({
	fileBlocks,
	sendTurn,
	overrides,
}) => {
	const [store] = useState(createChatStore);
	const chatInputRef = useRef<ChatMessageInputRef | null>(null);
	const inputValueRef = useRef("");
	const { isPending, mutateAsync } = useMutation({ mutationFn: sendTurn });
	const editing = useConversationEditingState({
		chatID: "chat-history-edit",
		onSend: mutateAsync,
		chatInputRef,
		inputValueRef,
	});
	return (
		<>
			<button
				type="button"
				onClick={() => editing.handleEditUserMessage(1, "edited", fileBlocks)}
			>
				Edit message
			</button>
			<ChatPageInput
				{...defaultChatPageInputProps(store)}
				{...overrides}
				onSend={editing.handleSendFromInput}
				isSendPending={isPending}
				isEditing={editing.editingMessageId !== null}
				editingFileBlocks={editing.editingFileBlocks}
				inputRef={chatInputRef}
				initialValue={editing.editorInitialValue}
				remountKey={editing.remountKey}
				onContentChange={editing.handleContentChange}
				onCancelHistoryEdit={editing.handleCancelHistoryEdit}
			/>
		</>
	);
};

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
	beforeAll(() => {
		Object.defineProperty(Range.prototype, "getBoundingClientRect", {
			configurable: true,
			value: () => new DOMRect(0, 0, 1, 16),
		});
	});

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
			expect.objectContaining({
				fileId: "delivered-zip",
				mediaType: "application/zip",
				purpose: "workspace",
			}),
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
			expect.objectContaining({
				fileId: "staged-zip",
				mediaType: "application/zip",
				purpose: "workspace",
			}),
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
	it("keeps workspace attachment edits after a failed save", async () => {
		const user = userEvent.setup();
		vi.spyOn(API.experimental, "uploadChatFile").mockResolvedValue({
			id: "staged-zip",
		});
		const sendTurn = vi
			.fn<(options: SendChatTurnOptions) => Promise<void>>()
			.mockRejectedValueOnce(new Error("edit failed"))
			.mockResolvedValue(undefined);
		const mockRemovedAttachment = {
			...MockChatWorkspaceAttachmentPart,
			file_id: "removed-zip",
			name: "removed.zip",
		};
		const mockKeptAttachment = {
			...MockChatWorkspaceAttachmentPart,
			file_id: "kept-zip",
			name: "kept.zip",
		};

		renderWithAuth(
			<HistoryEditHarness
				fileBlocks={[mockRemovedAttachment, mockKeptAttachment]}
				sendTurn={sendTurn}
				overrides={{
					chat: {
						...MockChat,
						id: "chat-history-edit",
						workspace_id: MockWorkspace.id,
					},
					workspace: MockWorkspace,
				}}
			/>,
		);
		await user.click(
			await screen.findByRole("button", { name: "Edit message" }),
		);
		await user.click(
			await screen.findByRole("button", { name: "Remove removed.zip" }),
		);
		await user.upload(
			await screen.findByTestId("chat-attachment-file-input"),
			new File(["PK"], "added.zip", { type: "application/zip" }),
		);
		await user.click(await screen.findByRole("button", { name: "Save Edit" }));
		await waitFor(() => expect(sendTurn).toHaveBeenCalledTimes(1));
		await user.click(await screen.findByRole("button", { name: "Save Edit" }));

		await waitFor(() => expect(sendTurn).toHaveBeenCalledTimes(2));
		expect(sendTurn.mock.calls[1][0]).toMatchObject({
			editedMessageID: 1,
			attachments: [
				expect.objectContaining({
					fileId: mockKeptAttachment.file_id,
					purpose: "workspace",
				}),
				expect.objectContaining({ fileId: "staged-zip", purpose: "workspace" }),
			],
		});
	});
});
