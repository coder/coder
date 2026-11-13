import { act, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
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
import { toast } from "#/components/Toaster/toast";
import { MockChat, MockChatQueuedMessage } from "#/testHelpers/chatEntities";
import {
	MockChatModel,
	MockChatModelProviderDescriptor,
} from "#/testHelpers/chatModels";
import { createDeferred } from "#/testHelpers/deferred";
import {
	MockUserPreferenceSettings,
	mockApiError,
} from "#/testHelpers/entities";
import { renderWithAuth } from "#/testHelpers/renderHelpers";
import { MockSkill } from "#/testHelpers/skills";
import {
	createMockWebSocket,
	type MockWebSocketServer,
} from "#/testHelpers/websockets";
import { OneWayWebSocket } from "#/utils/OneWayWebSocket";
import type { AgentsPageOutletContext } from "../AgentsPageLayout";
import { parseStoredDraft } from "../utils/draftStorage";
import AgentChatPage from "./AgentChatPage";

const mockProjectChat = { ...MockChat, project_id: "project-1" };

const outletContext: Partial<AgentsPageOutletContext> = {
	chatErrorReasons: {},
	setChatErrorReason: vi.fn(),
	clearChatErrorReason: vi.fn(),
	onChatReady: vi.fn(),
};

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

const renderChatPage = (
	queuedMessages: readonly TypesGen.ChatQueuedMessage[] = [],
) => {
	vi.spyOn(apiModule, "watchChat").mockImplementation(
		() =>
			new OneWayWebSocket({
				apiRoute: `/api/experimental/chats/${mockProjectChat.id}/stream`,
				websocketInit: (url, protocol) => createMockWebSocket(url, protocol)[0],
			}),
	);
	vi.spyOn(API.experimental, "getChat").mockResolvedValue(mockProjectChat);
	vi.spyOn(API.experimental, "getChatMessages").mockResolvedValue({
		messages: [],
		queued_messages: queuedMessages,
		has_more: false,
		snapshot_version: 1,
	});
	vi.spyOn(API.experimental, "getChatModels").mockResolvedValue({
		models: [
			{
				...MockChatModel,
				id: mockProjectChat.last_model_config_id,
				organization_id: mockProjectChat.organization_id,
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
	return renderWithAuth(<Outlet context={outletContext} />, {
		path: "/agents",
		route: `/agents/${mockProjectChat.id}`,
		children: [{ path: ":agentId", element: <AgentChatPage /> }],
	});
};

describe("AgentChatPage", () => {
	it("refreshes the project actions after a send fails", async () => {
		const getChatProjectSkills = vi
			.spyOn(API.experimental, "getChatProjectSkills")
			.mockResolvedValue([{ ...MockSkill, name: "deploy", action: true }]);
		const createChatMessage = vi
			.spyOn(API.experimental, "createChatMessage")
			.mockRejectedValue(
				mockApiError({ message: 'Skill "deploy" is not available.' }),
			);
		const user = userEvent.setup();

		const { queryClient } = renderChatPage();

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
					screen.getByRole("button", { name: "Save Edit" }),
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

		renderChatPage([MockChatQueuedMessage]);

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
});
