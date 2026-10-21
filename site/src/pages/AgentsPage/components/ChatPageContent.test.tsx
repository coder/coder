import { act, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http } from "msw";
import { useRef } from "react";
import {
	defaultScheduler,
	notifyManager,
	useMutation,
	useQueryClient,
} from "react-query";
import {
	afterAll,
	afterEach,
	beforeAll,
	beforeEach,
	describe,
	expect,
	it,
	vi,
} from "vitest";
import { API } from "#/api/api";
import { editChatMessage } from "#/api/queries/chats";
import type * as TypesGen from "#/api/typesGenerated";
import {
	MockChat,
	MockChatMessage,
	MockChatWorkspaceAttachmentPart,
} from "#/testHelpers/chatEntities";
import { MockWorkspace, MockWorkspaceAgent } from "#/testHelpers/entities";
import { renderWithAuth } from "#/testHelpers/renderHelpers";
import { server } from "#/testHelpers/server";
import { useConversationEditingState } from "../hooks/useConversationEditingState";
import type { ChatMessageInputRef } from "./AgentChatInput";
import { createChatStore } from "./ChatConversation/chatStore";
import { submitChatTurn } from "./ChatConversation/submitChatTurn";
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

const historyEditChatId = "chat-history-edit";

type HistoryEditHarnessProps = {
	store: ReturnType<typeof createChatStore>;
	fileBlocks: readonly TypesGen.ChatMessagePart[];
	overrides: Partial<ChatPageInputProps>;
};

const HistoryEditHarness: React.FC<HistoryEditHarnessProps> = ({
	store,
	fileBlocks,
	overrides,
}) => {
	const chatInputRef = useRef<ChatMessageInputRef | null>(null);
	const inputValueRef = useRef("");
	const isEditReasoningEffortDirtyRef = useRef(false);
	const queryClient = useQueryClient();
	const { isPending, mutateAsync: editMessage } = useMutation(
		editChatMessage(queryClient, historyEditChatId),
	);
	const editing = useConversationEditingState({
		chatID: historyEditChatId,
		onSend: (options) =>
			submitChatTurn({
				...options,
				composerParts: chatInputRef.current?.getContentParts() ?? [],
				isSubmissionPending: isPending,
				hasModelOptions: true,
				isEditReasoningEffortDirtyRef,
				personalSkills: [],
				organizationSkills: [],
				workspaceSkills: [],
				compact: vi.fn(),
				clearChatContext: vi.fn(),
				store,
				agentId: historyEditChatId,
				clearChatErrorReason: vi.fn(),
				acceptServerChatStatus: vi.fn(),
				chatMessages: [],
				effectiveSelectedModel: "model-config-1",
				modelOptions: defaultChatPageInputProps(store).modelOptions,
				effectiveReasoningEffort: undefined,
				mcpServerIds: [],
				editMessage,
				sendMessage: vi.fn(),
				onRequestError: vi.fn(),
				invalidateChat: vi.fn(),
				scrollToEnd: vi.fn(),
				upsertCacheMessages: vi.fn(),
				getCacheQueuedMessages: () => undefined,
				setCacheQueuedMessages: vi.fn(),
				fetchQueueConvergence: vi.fn(),
				setCachedChatPlanMode: vi.fn(),
			}),
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

// Records the content of every history edit save. The first save gets
// firstResponse when given; every other save succeeds.
const recordEditRequests = (firstResponse?: () => Response) => {
	const savedContent: TypesGen.ChatInputPart[][] = [];
	server.use(
		http.patch(
			"/api/v2/chats/:chatId/messages/:messageId",
			async ({ request }) => {
				const body = (await request.json()) as TypesGen.EditChatMessageRequest;
				savedContent.push([...body.content]);
				if (firstResponse && savedContent.length === 1) {
					return firstResponse();
				}
				return HttpResponse.json({
					message: { ...MockChatMessage, chat_id: historyEditChatId },
				});
			},
		),
	);
	return savedContent;
};

const failedEditResponses = [
	{ name: "a network error", respond: () => HttpResponse.error() },
	{
		name: "an HTTP 500",
		respond: () =>
			HttpResponse.json({ message: "Internal error." }, { status: 500 }),
	},
];

describe("ChatPageInput", () => {
	// jsdom's Range has no getBoundingClientRect for vi.spyOn to wrap, and
	// Lexical calls it when the composer regains focus.
	const rangeRectDescriptor = Object.getOwnPropertyDescriptor(
		Range.prototype,
		"getBoundingClientRect",
	);
	beforeAll(() => {
		Object.defineProperty(Range.prototype, "getBoundingClientRect", {
			configurable: true,
			value: () => new DOMRect(0, 0, 1, 16),
		});
	});
	afterAll(() => {
		if (rangeRectDescriptor) {
			Object.defineProperty(
				Range.prototype,
				"getBoundingClientRect",
				rangeRectDescriptor,
			);
		} else {
			Reflect.deleteProperty(Range.prototype, "getBoundingClientRect");
		}
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

	describe.each(failedEditResponses)(
		"history edit retry after $name",
		({ respond }) => {
			// jsdom applies the rollback before TanStack's setTimeout notification,
			// but browsers can render the settled mutation first. A microtask
			// scheduler forces that browser order.
			beforeEach(() => {
				notifyManager.setScheduler(queueMicrotask);
			});
			afterEach(() => {
				notifyManager.setScheduler(defaultScheduler);
			});

			const saveTwice = async (
				user: ReturnType<typeof userEvent.setup>,
				savedContent: readonly TypesGen.ChatInputPart[][],
			) => {
				await user.click(
					await screen.findByRole("button", { name: "Save Edit" }),
				);
				await waitFor(() => expect(savedContent).toHaveLength(1));
				await user.click(
					await screen.findByRole("button", { name: "Save Edit" }),
				);
				await waitFor(() => expect(savedContent).toHaveLength(2));
			};

			it("keeps staged workspace attachment edits", async () => {
				const user = userEvent.setup();
				const savedContent = recordEditRequests(respond);
				vi.spyOn(API.experimental, "uploadChatFile").mockResolvedValue({
					id: "staged-zip",
				});
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
						store={createChatStore()}
						fileBlocks={[mockRemovedAttachment, mockKeptAttachment]}
						overrides={{
							chat: {
								...MockChat,
								id: historyEditChatId,
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
				await saveTwice(user, savedContent);

				expect(savedContent[1]).toEqual(savedContent[0]);
				expect(savedContent[1]).toEqual([
					{ type: "text", text: "edited" },
					{ type: "workspace-attachment", file_id: mockKeptAttachment.file_id },
					{ type: "workspace-attachment", file_id: "staged-zip" },
				]);
			});

			it("keeps streamed workspace file edits", async () => {
				const user = userEvent.setup();
				const savedContent = recordEditRequests(respond);
				vi.spyOn(API.experimental, "uploadChatWorkspaceFile").mockResolvedValue(
					{
						path: "/home/coder/added.zip",
						name: "added.zip",
						size: 2,
						media_type: "application/zip",
						workspace_id: MockWorkspace.id,
					},
				);
				renderWithAuth(
					<HistoryEditHarness
						store={createChatStore()}
						fileBlocks={[
							workspaceFileReference("kept.csv", MockWorkspace.id),
							workspaceFileReference("removed.csv", MockWorkspace.id),
						]}
						overrides={{
							chat: {
								...MockChat,
								id: historyEditChatId,
								workspace_id: MockWorkspace.id,
								agent_id: MockWorkspaceAgent.id,
							},
							workspace: MockWorkspace,
							workspaceAgent: MockWorkspaceAgent,
						}}
					/>,
				);

				await user.click(
					await screen.findByRole("button", { name: "Edit message" }),
				);
				await user.click(
					await screen.findByRole("button", { name: "Remove removed.csv" }),
				);
				await user.upload(
					await screen.findByTestId("chat-attachment-file-input"),
					new File(["PK"], "added.zip", { type: "application/zip" }),
				);
				await saveTwice(user, savedContent);

				expect(savedContent[1]).toEqual(savedContent[0]);
				expect(savedContent[1]).toEqual([
					{ type: "text", text: "edited" },
					expect.objectContaining({ workspace_file_name: "kept.csv" }),
					expect.objectContaining({ workspace_file_name: "added.zip" }),
				]);
			});

			it("keeps prompt image edits", async () => {
				const user = userEvent.setup();
				const savedContent = recordEditRequests(respond);
				vi.spyOn(API.experimental, "uploadChatFile").mockResolvedValue({
					id: "added-png",
				});
				const mockImagePart: TypesGen.ChatFilePart = {
					type: "file",
					file_id: "image-png",
					media_type: "image/png",
				};
				const mockRemovedImagePart = {
					...mockImagePart,
					file_id: "removed-png",
				};
				const mockKeptImagePart = { ...mockImagePart, file_id: "kept-png" };
				renderWithAuth(
					<HistoryEditHarness
						store={createChatStore()}
						fileBlocks={[mockRemovedImagePart, mockKeptImagePart]}
						overrides={{ chat: { ...MockChat, id: historyEditChatId } }}
					/>,
				);

				await user.click(
					await screen.findByRole("button", { name: "Edit message" }),
				);
				await user.click(
					await screen.findByRole("button", {
						name: "Remove attachment-0.png",
					}),
				);
				await user.upload(
					await screen.findByTestId("chat-attachment-file-input"),
					new File(["png"], "added.png", { type: "image/png" }),
				);
				await saveTwice(user, savedContent);

				expect(savedContent[1]).toEqual(savedContent[0]);
				expect(savedContent[1]).toEqual([
					{ type: "text", text: "edited" },
					{ type: "file", file_id: mockKeptImagePart.file_id },
					{ type: "file", file_id: "added-png" },
				]);
			});
		},
	);

	type LeaveEditContext = {
		user: ReturnType<typeof userEvent.setup>;
		store: ReturnType<typeof createChatStore>;
		savedContent: readonly TypesGen.ChatInputPart[][];
	};

	it.each([
		{
			name: "canceling",
			leaveEdit: async ({ user }: LeaveEditContext) => {
				await user.click(
					screen.getByRole("button", { name: "Cancel editing" }),
				);
			},
		},
		{
			name: "saving",
			leaveEdit: async ({ user, store, savedContent }: LeaveEditContext) => {
				await user.click(screen.getByRole("button", { name: "Save Edit" }));
				await waitFor(() => expect(savedContent).toHaveLength(1));
				// The composer re-enables once the save settles, after its cleanup.
				await waitFor(() =>
					expect(screen.getByRole("textbox")).toHaveAttribute(
						"aria-disabled",
						"false",
					),
				);
				// The edit restarts the chat; finish that turn so the message can
				// be edited again.
				act(() => store.setChatStatus("waiting"));
			},
		},
	])("drops attachment edits after $name the edit", async ({ leaveEdit }) => {
		const user = userEvent.setup();
		const store = createChatStore();
		const savedContent = recordEditRequests();
		vi.spyOn(API.experimental, "uploadChatFile").mockResolvedValue({
			id: "staged-zip",
		});
		const mockOriginalAttachment = {
			...MockChatWorkspaceAttachmentPart,
			file_id: "original-zip",
			name: "original.zip",
		};
		renderWithAuth(
			<HistoryEditHarness
				store={store}
				fileBlocks={[mockOriginalAttachment]}
				overrides={{
					chat: {
						...MockChat,
						id: historyEditChatId,
						workspace_id: MockWorkspace.id,
					},
					workspace: MockWorkspace,
				}}
			/>,
		);

		const editButton = await screen.findByRole("button", {
			name: "Edit message",
		});
		await user.click(editButton);
		await user.click(
			await screen.findByRole("button", { name: "Remove original.zip" }),
		);
		await user.upload(
			await screen.findByTestId("chat-attachment-file-input"),
			new File(["PK"], "added.zip", { type: "application/zip" }),
		);
		await leaveEdit({ user, store, savedContent });
		const leaveSaveCount = savedContent.length;

		await user.click(editButton);
		await user.click(await screen.findByRole("button", { name: "Save Edit" }));
		await waitFor(() => expect(savedContent).toHaveLength(leaveSaveCount + 1));
		expect(savedContent.at(-1)).toEqual([
			{ type: "text", text: "edited" },
			{ type: "workspace-attachment", file_id: mockOriginalAttachment.file_id },
		]);
	});
});
