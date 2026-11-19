import { act, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http } from "msw";
import { defaultScheduler, notifyManager } from "react-query";
import { Outlet } from "react-router";
import {
	afterEach,
	beforeAll,
	describe,
	expect,
	it,
	onTestFinished,
	vi,
} from "vitest";
import * as apiModule from "#/api/api";
import { API } from "#/api/api";
import { chatProjectSkillsKey } from "#/api/queries/chatProjectSkills";
import type * as TypesGen from "#/api/typesGenerated";
import type { Chat } from "#/api/typesGenerated";
import { toast } from "#/components/Toaster/toast";
import { AGENT_BROWSER_APP_SLUG } from "#/modules/apps/apps";
import {
	MockChat,
	MockChatDiffStatus,
	MockChatMessage,
	MockChatQueuedMessage,
	MockOpenPreviewResult,
	MockPreviewSignInResponse,
} from "#/testHelpers/chatEntities";
import {
	MockChatModel,
	MockChatModelProviderDescriptor,
} from "#/testHelpers/chatModels";
import { createDeferred } from "#/testHelpers/deferred";
import {
	MockUserPreferenceSettings,
	MockWorkspace,
	MockWorkspaceAgent,
	MockWorkspaceApp,
	MockWorkspaceResource,
	mockApiError,
} from "#/testHelpers/entities";
import { renderWithAuth } from "#/testHelpers/renderHelpers";
import { server } from "#/testHelpers/server";
import { MockSkill } from "#/testHelpers/skills";
import {
	createMockWebSocket,
	type MockWebSocketServer,
} from "#/testHelpers/websockets";
import { OneWayWebSocket } from "#/utils/OneWayWebSocket";
import type { AgentsPageOutletContext } from "../AgentsPageLayout";
import { RIGHT_PANEL_OPEN_KEY } from "../components/RightPanel/RightPanel";
import { parseStoredDraft } from "../utils/draftStorage";
import { overviewCardOpenStorageKey } from "../utils/overviewCardStorage";
import {
	getPersistedRightPanelTabs,
	savePersistedDefaultTerminalHidden,
	savePersistedRightPanelTabs,
} from "../utils/rightPanelTabStorage";
import type { NewFrameTab } from "../utils/rightPanelTabs";
import { getPersistedSidebarTabId } from "../utils/sidebarTabStorage";
import AgentChatPage from "./AgentChatPage";

const mockProjectChat = { ...MockChat, project_id: "project-1" };

const outletContext: Partial<AgentsPageOutletContext> = {
	chatErrorReasons: {},
	setChatErrorReason: vi.fn(),
	clearChatErrorReason: vi.fn(),
	onChatReady: vi.fn(),
};

const mockChatPageApi = (
	chat: Chat,
	queuedMessages: readonly TypesGen.ChatQueuedMessage[] = [],
) => {
	vi.spyOn(apiModule, "watchChat").mockImplementation(
		() =>
			new OneWayWebSocket({
				apiRoute: `/api/experimental/chats/${chat.id}/stream`,
				websocketInit: (url, protocol) => createMockWebSocket(url, protocol)[0],
			}),
	);
	vi.spyOn(API.experimental, "getChat").mockResolvedValue(chat);
	vi.spyOn(API.experimental, "getChatMessages").mockResolvedValue({
		messages: [],
		queued_messages: [...queuedMessages],
		has_more: false,
		snapshot_version: 1,
	});
	vi.spyOn(API.experimental, "getChatModels").mockResolvedValue({
		models: [
			{
				...MockChatModel,
				id: chat.last_model_config_id,
				organization_id: chat.organization_id,
				is_default: true,
			},
		],
		providers: [MockChatModelProviderDescriptor],
		unsupported_providers: [],
	});
	vi.spyOn(API, "getUserPreferenceSettings").mockResolvedValue(
		MockUserPreferenceSettings,
	);
	vi.spyOn(API, "checkAuthorization").mockImplementation(async ({ checks }) =>
		Object.fromEntries(Object.keys(checks).map((key) => [key, true])),
	);
	vi.spyOn(API.experimental, "getMCPServerConfigs").mockResolvedValue([]);
};

/** The transcript has messages before the loaded page. */
const mockOlderMessages = () =>
	vi.spyOn(API.experimental, "getChatMessages").mockResolvedValue({
		messages: [MockChatMessage],
		queued_messages: [],
		has_more: true,
	});

const renderChatPage = (chat: Chat) =>
	renderWithAuth(<Outlet context={outletContext} />, {
		path: "/agents",
		route: `/agents/${chat.id}`,
		children: [{ path: ":agentId", element: <AgentChatPage /> }],
	});

/**
 * Waits for the loaded chat. The loading view renders its own overview toggle
 * and a skeleton card with the same names, and unmounts them when the chat
 * loads.
 */
const waitForLoadedChat = () =>
	screen.findByRole("region", { name: "Messages" });

afterEach(() => {
	vi.restoreAllMocks();
	localStorage.clear();
});

beforeAll(() => {
	// Lexical measures the selection to scroll it into view; jsdom has no
	// layout.
	Object.defineProperty(Range.prototype, "getBoundingClientRect", {
		configurable: true,
		value: () => new DOMRect(0, 0, 1, 16),
	});
});

// Renders the page for MockChat with the given queued rows; the user owns
// the chat. publish delivers stream events to the page. Queries are scoped
// to this render, so a test can render the page twice as two tabs.
const renderQueuedChat = ({
	queuedMessages,
	chat = MockChat,
	models = [{ ...MockChatModel, id: chat.last_model_config_id }],
}: {
	queuedMessages: readonly TypesGen.ChatQueuedMessage[];
	chat?: TypesGen.Chat;
	models?: readonly TypesGen.ChatModel[];
}) => {
	let server: MockWebSocketServer | undefined;
	vi.spyOn(apiModule, "watchChat").mockImplementation(
		() =>
			new OneWayWebSocket({
				apiRoute: `/api/experimental/chats/${chat.id}/stream`,
				websocketInit: (url, protocol) => {
					const [socket, socketServer] = createMockWebSocket(url, protocol);
					server = socketServer;
					return socket;
				},
			}),
	);
	vi.spyOn(API.experimental, "getChat").mockResolvedValue(chat);
	vi.spyOn(API.experimental, "getChatMessages").mockResolvedValue({
		messages: [],
		queued_messages: [...queuedMessages],
		has_more: false,
		snapshot_version: 1,
	});
	vi.spyOn(API.experimental, "getChatModels").mockResolvedValue({
		models: models.map((model) => ({
			...model,
			organization_id: chat.organization_id,
		})),
		providers: [MockChatModelProviderDescriptor],
		unsupported_providers: [],
	});
	vi.spyOn(API, "getUserPreferenceSettings").mockResolvedValue(
		MockUserPreferenceSettings,
	);
	vi.spyOn(API, "checkAuthorization").mockImplementation(async ({ checks }) =>
		Object.fromEntries(Object.keys(checks).map((key) => [key, true])),
	);
	vi.spyOn(API.experimental, "getMCPServerConfigs").mockResolvedValue([]);
	const editChatQueuedMessage = vi
		.spyOn(API.experimental, "editChatQueuedMessage")
		.mockResolvedValue(undefined);
	const deleteChatQueuedMessage = vi
		.spyOn(API.experimental, "deleteChatQueuedMessage")
		.mockResolvedValue(undefined);
	const promoteChatQueuedMessage = vi
		.spyOn(API.experimental, "promoteChatQueuedMessage")
		.mockResolvedValue(undefined);
	const user = userEvent.setup();

	const { container, unmount } = renderWithAuth(
		<Outlet context={outletContext} />,
		{
			path: "/agents",
			route: `/agents/${chat.id}`,
			children: [{ path: ":agentId", element: <AgentChatPage /> }],
		},
	);
	const view = within(container);

	const composer = () => view.getByRole("textbox", { name: "Chat message" });
	// Lexical does not handle userEvent keystrokes in jsdom; a paste goes
	// through its input path.
	const type = async (text: string) => {
		await user.click(
			await view.findByRole("textbox", { name: "Chat message" }),
		);
		await user.paste(text);
	};
	// The tests render one queued row, so its actions have unique names.
	const rowAction = async (action: string) => {
		await user.click(await view.findByRole("button", { name: action }));
	};
	const publish = (event: TypesGen.ChatStreamEvent) =>
		act(() =>
			server?.publishMessage(
				new MessageEvent("message", { data: JSON.stringify([event]) }),
			),
		);
	return {
		user,
		view,
		unmount,
		composer,
		type,
		rowAction,
		publish,
		editChatQueuedMessage,
		deleteChatQueuedMessage,
		promoteChatQueuedMessage,
	};
};

describe("AgentChatPage", () => {
	it("refreshes the project actions after a send fails", async () => {
		mockChatPageApi(mockProjectChat);
		const getChatProjectSkills = vi
			.spyOn(API.experimental, "getChatProjectSkills")
			.mockResolvedValue([{ ...MockSkill, name: "deploy", action: true }]);
		const createChatMessage = vi
			.spyOn(API.experimental, "createChatMessage")
			.mockRejectedValue(
				mockApiError({ message: 'Skill "deploy" is not available.' }),
			);
		const user = userEvent.setup();

		const { queryClient } = renderChatPage(mockProjectChat);

		// The actions bar remounts after the send, so only the failure's
		// invalidation may refetch the list.
		queryClient.setQueryDefaults(
			chatProjectSkillsKey(mockProjectChat.project_id),
			{ staleTime: Number.POSITIVE_INFINITY },
		);
		await user.click(await screen.findByRole("button", { name: "deploy" }));
		// The page and the actions bar share the list, so let both settle.
		await waitFor(() =>
			expect(
				queryClient.isFetching({
					queryKey: chatProjectSkillsKey(mockProjectChat.project_id),
				}),
			).toBe(0),
		);
		const listRequestsBeforeSend = getChatProjectSkills.mock.calls.length;
		await user.click(screen.getByRole("button", { name: "Send" }));

		await waitFor(() => expect(createChatMessage).toHaveBeenCalledTimes(1));
		await waitFor(() =>
			expect(getChatProjectSkills).toHaveBeenCalledTimes(
				listRequestsBeforeSend + 1,
			),
		);
	});

	it.each(["resolves", "rejects"] as const)(
		"starts a send only after an earlier queue request %s",
		async (outcome) => {
			const page = renderQueuedChat({
				queuedMessages: [{ ...MockChatQueuedMessage, chat_id: MockChat.id }],
			});
			const deleted = createDeferred<undefined>();
			page.deleteChatQueuedMessage.mockReturnValue(deleted.promise);
			const createChatMessage = vi
				.spyOn(API.experimental, "createChatMessage")
				.mockResolvedValue({ queued: false });

			await page.rowAction("Remove from queue");
			await page.type("hello");
			await page.user.click(screen.getByRole("button", { name: "Send" }));
			await waitFor(() =>
				expect(page.deleteChatQueuedMessage).toHaveBeenCalledTimes(1),
			);
			expect(createChatMessage).not.toHaveBeenCalled();

			act(() => {
				if (outcome === "resolves") {
					deleted.resolve(undefined);
				} else {
					deleted.reject(mockApiError({ message: "Internal error." }));
				}
			});
			await waitFor(() => expect(createChatMessage).toHaveBeenCalledTimes(1));
		},
	);

	it("shows the overview card in a new chat and keeps the toggle's choice", async () => {
		mockChatPageApi(MockChat);
		const user = userEvent.setup();
		const firstPage = renderChatPage(MockChat);
		await waitForLoadedChat();

		await user.click(
			await screen.findByRole("button", {
				name: "Toggle overview",
				expanded: true,
			}),
		);
		expect(localStorage.getItem(overviewCardOpenStorageKey)).toBe("false");

		firstPage.unmount();
		renderChatPage(MockChat);
		await waitForLoadedChat();
		await screen.findByRole("button", {
			name: "Toggle overview",
			expanded: false,
		});
	});

	it("focuses a row's panel tab and hides the overview card while the panel is open", async () => {
		const chat = { ...MockChat, diff_statuses: [MockChatDiffStatus] };
		mockChatPageApi(chat);
		const user = userEvent.setup();
		renderChatPage(chat);
		await waitForLoadedChat();

		const card = await screen.findByRole("complementary", {
			name: "Overview",
		});
		await user.click(within(card).getByRole("button", { name: /#123/ }));
		expect(screen.getByRole("tab", { name: "Git" })).toHaveFocus();
		expect(
			screen.queryByRole("complementary", { name: "Overview" }),
		).toBeNull();

		await user.click(
			screen.getByRole("button", { name: "Toggle panel", expanded: true }),
		);
		screen.getByRole("button", { name: "Toggle panel", expanded: false });
		await screen.findByRole("button", {
			name: "Toggle overview",
			expanded: true,
		});
		expect(localStorage.getItem(overviewCardOpenStorageKey)).toBeNull();
	});

	it("retries the overview card's earlier attachments after a failure", async () => {
		mockChatPageApi(MockChat);
		mockOlderMessages();
		const getChatAttachments = vi
			.spyOn(API.experimental, "getChatAttachments")
			.mockRejectedValueOnce(new Error("Failed to get chat attachments."))
			.mockResolvedValueOnce({ attachments: [] });
		const user = userEvent.setup();
		renderChatPage(MockChat);
		await waitForLoadedChat();

		const card = await screen.findByRole("complementary", {
			name: "Overview",
		});
		await user.click(
			await within(card).findByRole("button", { name: "Retry" }),
		);

		await waitFor(() => expect(getChatAttachments).toHaveBeenCalledTimes(2));
	});

	describe("queued message editing", () => {
		const row = {
			...MockChatQueuedMessage,
			id: 7,
			chat_id: MockChat.id,
			content: [{ type: "text" as const, text: "queued text" }],
		};
		const draftKey = `agents.draft-input.${MockChat.id}`;
		const savedEditKey = `${draftKey}.queued.${row.id}`;

		// Opens the row's edit and types into it, so the typed text is saved.
		const editAndType = async (page: ReturnType<typeof renderQueuedChat>) => {
			await page.rowAction("Edit");
			await waitFor(() =>
				expect(page.composer().textContent).toBe("queued text"),
			);
			await page.type(" edit");
			await waitFor(() =>
				expect(localStorage.getItem(savedEditKey)).not.toBeNull(),
			);
		};

		// Tab b renders after tab a typed into its edit, so b follows the edit
		// and loads the typed text.
		it.each([
			["gives its own draft back", ""],
			["keeps text it typed there as a draft", " more"],
		])(
			"when another tab's edit ends, a tab that followed it %s",
			async (_name, typedInB) => {
				const a = renderQueuedChat({ queuedMessages: [row] });
				await a.view.findByRole("button", { name: "Send now" });
				await a.type("draft");
				await editAndType(a);

				const b = renderQueuedChat({
					queuedMessages: [{ ...row, editing_since: "2024-01-01T00:00:00Z" }],
				});
				await b.view.findByRole("button", { name: "Editing" });
				await waitFor(() =>
					expect(b.composer().textContent).toBe("queued text edit"),
				);
				if (typedInB) {
					await b.type(typedInB);
				}
				await b.publish({
					type: "queue_update",
					chat_id: MockChat.id,
					queued_messages: [row],
					snapshot_version: 2,
				});

				const expected = typedInB ? `queued text edit${typedInB}` : "draft";
				await waitFor(() => expect(b.composer().textContent).toBe(expected));
				expect(parseStoredDraft(localStorage.getItem(draftKey)).text).toBe(
					expected,
				);
				expect(localStorage.getItem(savedEditKey)).toBeNull();
			},
		);

		it.each([
			[
				"restores the typed text while the row is unchanged",
				"queued text",
				"queued text edit",
			],
			[
				"loads the row's content when another client saved it since the text was typed",
				"saved elsewhere",
				"saved elsewhere",
			],
		])("Edit after a reload %s", async (_name, rowText, expected) => {
			const before = renderQueuedChat({ queuedMessages: [row] });
			await editAndType(before);
			before.unmount();

			const after = renderQueuedChat({
				queuedMessages: [
					{ ...row, content: [{ type: "text", text: rowText }] },
				],
			});
			await after.rowAction("Edit");

			await waitFor(() => expect(after.composer().textContent).toBe(expected));
			expect(localStorage.getItem(savedEditKey) !== null).toBe(
				rowText === "queued text",
			);
		});

		it.each([
			["Remove from queue", "deleteChatQueuedMessage"],
			["Send now", "promoteChatQueuedMessage"],
		] as const)(
			"%s on the row being edited discards the typed text and gives the draft back",
			async (action, request) => {
				const page = renderQueuedChat({ queuedMessages: [row] });
				await screen.findByRole("button", { name: "Send now" });
				await page.type("draft");
				expect(page.composer().textContent).toBe("draft");
				await editAndType(page);

				await page.rowAction(action);

				await waitFor(() =>
					expect(page[request]).toHaveBeenCalledWith(MockChat.id, row.id),
				);
				await waitFor(() => expect(page.composer().textContent).toBe("draft"));
				expect(page.editChatQueuedMessage).not.toHaveBeenCalledWith(
					MockChat.id,
					row.id,
					expect.objectContaining({ content: expect.anything() }),
				);
				expect(localStorage.getItem(savedEditKey)).toBeNull();
			},
		);

		// The row was queued with another model than the one the picker shows.
		it.each([
			[
				"keeps the row's model when the picker is untouched",
				"model-b",
				undefined,
				undefined,
			],
			[
				"sends the model picked during the edit",
				"model-b",
				"Model C",
				"model-c",
			],
			[
				"sends the picker's model when the row's model is no longer available",
				"model-deleted",
				undefined,
				MockChat.last_model_config_id,
			],
		])(
			"saving a queued edit %s",
			async (_name, rowModel, pickModelName, wantModelConfigID) => {
				const page = renderQueuedChat({
					queuedMessages: [{ ...row, model_config_id: rowModel }],
					models: [
						{
							...MockChatModel,
							id: MockChat.last_model_config_id,
							display_name: "Model A",
						},
						{ ...MockChatModel, id: "model-b", display_name: "Model B" },
						{ ...MockChatModel, id: "model-c", display_name: "Model C" },
					],
				});
				await page.rowAction("Edit");
				await waitFor(() =>
					expect(page.composer().textContent).toBe("queued text"),
				);
				if (pickModelName) {
					await page.user.click(
						screen.getByRole("combobox", { name: "Model A" }),
					);
					await page.user.click(
						within(await screen.findByRole("listbox")).getByRole("option", {
							name: new RegExp(pickModelName),
						}),
					);
				}
				await page.type(" edit");
				await page.user.click(
					screen.getByRole("button", { name: "Save edit" }),
				);

				await waitFor(() =>
					expect(page.editChatQueuedMessage).toHaveBeenCalledWith(
						MockChat.id,
						row.id,
						expect.objectContaining({
							content: expect.anything(),
							model_config_id: wantModelConfigID,
						}),
					),
				);
				expect(localStorage.getItem(savedEditKey)).toBeNull();
			},
		);

		// react-query delivers the failed request's state to the page on a
		// scheduler it owns, which may run before the request's own error
		// handlers. On a 404 the row is gone, and another client's edit of
		// another row follows.
		const otherRow = {
			...row,
			id: 8,
			content: [{ type: "text" as const, text: "other row" }],
		};
		it.each([
			["", defaultScheduler, 409, [row]],
			[
				" when the page sees the failure before the request's handlers",
				queueMicrotask,
				409,
				[row],
			],
			[" or of another row after a 404", defaultScheduler, 404, [otherRow]],
		])(
			"an Edit the server refuses leaves the composer as it was, so it does not open another client's edit of the row%s",
			async (_name, scheduler, status, queueAfter) => {
				notifyManager.setScheduler(scheduler);
				onTestFinished(() => notifyManager.setScheduler(defaultScheduler));
				const page = renderQueuedChat({ queuedMessages: [row] });
				await screen.findByRole("button", { name: "Send now" });
				await page.type("draft");
				// Cancelling an edit leaves the composer writing a new message.
				await page.rowAction("Edit");
				await waitFor(() =>
					expect(page.composer().textContent).toBe("queued text"),
				);
				await page.user.keyboard("{Escape}");
				await waitFor(() => expect(page.composer().textContent).toBe("draft"));

				page.editChatQueuedMessage.mockRejectedValueOnce({
					...mockApiError({ message: "Another edit is open." }),
					status,
				});
				await page.rowAction("Edit");
				await waitFor(() =>
					expect(page.editChatQueuedMessage).toHaveBeenCalledTimes(3),
				);
				await waitFor(() => expect(page.composer().textContent).toBe("draft"));

				await page.publish({
					type: "queue_update",
					chat_id: MockChat.id,
					queued_messages: queueAfter.map((queued) => ({
						...queued,
						editing_since: "2024-01-01T00:00:00Z",
					})),
					snapshot_version: 2,
				});
				await screen.findByRole("button", { name: "Editing" });
				expect(page.composer().textContent).toBe("draft");
			},
		);

		// The row leaves the queue only at a queue update, so Edit can begin
		// the edit again.
		it.each([
			[404, "Queued message was already sent or removed."],
			[409, "Another edit is open."],
		])(
			"an Edit refused with %i reports it and can be retried",
			async (status, message) => {
				const toastError = vi.spyOn(toast, "error");
				const page = renderQueuedChat({ queuedMessages: [row] });
				page.editChatQueuedMessage.mockRejectedValueOnce({
					...mockApiError({ message: "Another edit is open." }),
					status,
				});

				await page.rowAction("Edit");
				await waitFor(() => expect(toastError).toHaveBeenCalledWith(message));
				await page.rowAction("Edit");

				await waitFor(() =>
					expect(page.editChatQueuedMessage).toHaveBeenCalledTimes(2),
				);
			},
		);

		it("reports a failed Remove from queue", async () => {
			const toastError = vi.spyOn(toast, "error");
			const page = renderQueuedChat({ queuedMessages: [row] });
			page.deleteChatQueuedMessage.mockRejectedValueOnce(
				mockApiError({ message: "Delete failed." }),
			);

			await page.rowAction("Remove from queue");

			await waitFor(() =>
				expect(toastError).toHaveBeenCalledWith("Delete failed."),
			);
		});
	});

	it("reports a failed promote and keeps the row queued", async () => {
		vi.spyOn(API.experimental, "getChatProjectSkills").mockResolvedValue([]);
		const promote = vi
			.spyOn(API.experimental, "promoteChatQueuedMessage")
			.mockRejectedValue(mockApiError({ message: "promote failed" }));
		const user = userEvent.setup();

		mockChatPageApi(mockProjectChat, [MockChatQueuedMessage]);
		renderChatPage(mockProjectChat);

		await user.click(await screen.findByRole("button", { name: "Send now" }));
		await waitFor(() =>
			expect(outletContext.setChatErrorReason).toHaveBeenCalledWith(
				mockProjectChat.id,
				expect.objectContaining({ message: "promote failed" }),
			),
		);
		// The row is listed again, so it can be promoted again.
		await user.click(screen.getByRole("button", { name: "Send now" }));
		await waitFor(() => expect(promote).toHaveBeenCalledTimes(2));
	});

	describe("open_preview results", () => {
		const mockCodeServerApp: TypesGen.WorkspaceApp = {
			...MockWorkspaceApp,
			id: "code-server-app",
			slug: "code-server",
			display_name: "code-server",
			subdomain: true,
			subdomain_name: "code-server--a-workspace-agent--test-workspace--admin",
		};
		const mockBrowserApp: TypesGen.WorkspaceApp = {
			...MockWorkspaceApp,
			id: "agent-browser-app",
			slug: AGENT_BROWSER_APP_SLUG,
			subdomain: true,
			subdomain_name: "agent-browser--a-workspace-agent--test-workspace--admin",
		};
		const mockPreviewAgent = {
			...MockWorkspaceAgent,
			apps: [mockCodeServerApp, mockBrowserApp, MockWorkspaceApp],
		};
		const mockPreviewWorkspace: TypesGen.Workspace = {
			...MockWorkspace,
			latest_build: {
				...MockWorkspace.latest_build,
				resources: [{ ...MockWorkspaceResource, agents: [mockPreviewAgent] }],
			},
		};
		const mockPreviewChat: Chat = {
			...MockChat,
			workspace_id: mockPreviewWorkspace.id,
			agent_id: mockPreviewAgent.id,
		};

		const previewExchange = (
			id: number,
			result: { title: string } & Record<string, unknown>,
		): TypesGen.ChatMessage[] => [
			{
				...MockChatMessage,
				chat_id: mockPreviewChat.id,
				id,
				role: "assistant",
				content: [
					{
						type: "tool-call",
						tool_call_id: `call-${id}`,
						tool_name: "open_preview",
						args: { title: result.title },
					},
				],
			},
			{
				...MockChatMessage,
				chat_id: mockPreviewChat.id,
				id: id + 1,
				role: "tool",
				content: [
					{
						type: "tool-result",
						tool_call_id: `call-${id}`,
						tool_name: "open_preview",
						result,
					},
				],
			},
		];

		const renderPreviewChat = ({
			messages = [],
		}: {
			messages?: readonly TypesGen.ChatMessage[];
		}) => {
			server.use(
				http.get("/api/v2/experiments", () =>
					HttpResponse.json(["open-preview"]),
				),
			);
			mockChatPageApi(mockPreviewChat);
			vi.spyOn(API.experimental, "getChatMessages").mockResolvedValue({
				messages: [...messages],
				queued_messages: [],
				has_more: false,
				snapshot_version: 1,
			});
			vi.spyOn(API, "getWorkspace").mockResolvedValue(mockPreviewWorkspace);
			vi.spyOn(apiModule, "watchWorkspace").mockImplementation(
				() =>
					new OneWayWebSocket({
						apiRoute: `/api/v2/workspaces/${mockPreviewWorkspace.id}/watch-ws`,
						websocketInit: (url, protocol) =>
							createMockWebSocket(url, protocol)[0],
					}),
			);
			const signIn = vi
				.spyOn(API.experimental, "signInToPreview")
				.mockResolvedValue(MockPreviewSignInResponse);
			savePersistedDefaultTerminalHidden(mockPreviewChat.id, true);
			renderChatPage(mockPreviewChat);
			return { user: userEvent.setup(), signIn };
		};

		it.each<{
			name: string;
			button: string;
			result: { title: string } & Record<string, unknown>;
			tab: NewFrameTab;
		}>([
			{
				name: "port",
				button: "Open preview",
				result: { ...MockOpenPreviewResult, url_path: "/docs?page=2" },
				tab: {
					kind: "preview",
					label: MockOpenPreviewResult.title,
					workspaceId: mockPreviewWorkspace.id,
					agentName: mockPreviewAgent.name,
					slug: MockOpenPreviewResult.preview_slug,
					urlPath: "/docs?page=2",
				},
			},
			{
				name: "file",
				button: "Open preview",
				result: {
					...MockOpenPreviewResult,
					kind: "file",
					port: undefined,
					preview_slug: "preview--0123abcd",
					url_path: "/?page=2",
				},
				tab: {
					kind: "preview",
					label: MockOpenPreviewResult.title,
					workspaceId: mockPreviewWorkspace.id,
					agentName: mockPreviewAgent.name,
					slug: "preview--0123abcd",
					urlPath: "/?page=2",
				},
			},
			{
				name: "app",
				button: "Open app",
				result: {
					...MockOpenPreviewResult,
					kind: "app",
					port: undefined,
					preview_slug: undefined,
					app_slug: mockCodeServerApp.slug,
					url_path: "/?folder=/home/coder",
				},
				tab: {
					kind: "workspace_app",
					label: "code-server",
					agentId: mockPreviewAgent.id,
					appId: mockCodeServerApp.id,
					urlPath: "/?folder=/home/coder",
				},
			},
		])(
			"opens a $name result's tab from its row once",
			async ({ button, result, tab }) => {
				const page = renderPreviewChat({
					messages: previewExchange(10, result),
				});
				const openButton = await screen.findByRole("button", {
					name: button,
				});

				await page.user.click(openButton);
				await page.user.click(openButton);

				await waitFor(() =>
					expect(getPersistedRightPanelTabs(mockPreviewChat.id)).toEqual([
						expect.objectContaining(tab),
					]),
				);
				expect(getPersistedSidebarTabId(mockPreviewChat.id)).toBe(
					getPersistedRightPanelTabs(mockPreviewChat.id)[0]?.id,
				);
				expect(screen.getByRole("tab", { name: tab.label })).toHaveFocus();
			},
		);

		it("opens the same preview at another URL path in its own tab", async () => {
			const page = renderPreviewChat({
				messages: [
					...previewExchange(10, MockOpenPreviewResult),
					...previewExchange(12, {
						...MockOpenPreviewResult,
						url_path: "/admin",
					}),
				],
			});
			const [rootButton, adminButton] = await screen.findAllByRole("button", {
				name: "Open preview",
			});

			await page.user.click(rootButton);
			await page.user.click(adminButton);

			await waitFor(() =>
				expect(
					getPersistedRightPanelTabs(mockPreviewChat.id).map((tab) =>
						tab.kind === "preview" ? tab.urlPath : tab.kind,
					),
				).toEqual([undefined, "/admin"]),
			);
		});

		it("opens the Browser tab for an agent-browser result", async () => {
			const page = renderPreviewChat({
				messages: previewExchange(10, {
					...MockOpenPreviewResult,
					kind: "app",
					port: undefined,
					preview_slug: undefined,
					app_slug: AGENT_BROWSER_APP_SLUG,
				}),
			});

			await page.user.click(
				await screen.findByRole("button", { name: "Open app" }),
			);

			await waitFor(() =>
				expect(getPersistedSidebarTabId(mockPreviewChat.id)).toBe("browser"),
			);
			expect(getPersistedRightPanelTabs(mockPreviewChat.id)).toEqual([]);
		});

		it("signs in to a restored preview tab once it is shown, and again on reload", async () => {
			localStorage.setItem(RIGHT_PANEL_OPEN_KEY, "true");
			savePersistedRightPanelTabs(mockPreviewChat.id, [
				{
					id: "preview-docs",
					kind: "preview",
					label: "Docs site",
					workspaceId: mockPreviewWorkspace.id,
					agentName: mockPreviewAgent.name,
					slug: "preview--3000",
					urlPath: "/docs",
				},
			]);
			const page = renderPreviewChat({});
			const previewTab = await screen.findByRole("tab", { name: "Docs site" });
			expect(page.signIn).not.toHaveBeenCalled();

			await page.user.click(previewTab);
			await waitFor(() => expect(page.signIn).toHaveBeenCalledTimes(1));
			await page.user.click(
				screen.getByRole("button", { name: "Reload preview" }),
			);

			await waitFor(() => expect(page.signIn).toHaveBeenCalledTimes(2));
			expect(page.signIn).toHaveBeenLastCalledWith(
				expect.objectContaining({
					workspace_id: mockPreviewWorkspace.id,
					agent_name: mockPreviewAgent.name,
					preview_slug: "preview--3000",
					url_path: "/docs",
				}),
			);
		});
	});
});
