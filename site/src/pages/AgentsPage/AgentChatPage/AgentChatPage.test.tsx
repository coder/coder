import { act, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { Outlet } from "react-router";
import { afterEach, beforeAll, describe, expect, it, vi } from "vitest";
import * as apiModule from "#/api/api";
import { API } from "#/api/api";
import { chatProjectSkillsKey } from "#/api/queries/chatProjectSkills";
import type * as TypesGen from "#/api/typesGenerated";
import { MockChat, MockChatQueuedMessage } from "#/testHelpers/chatEntities";
import {
	MockChatModel,
	MockChatModelProviderDescriptor,
} from "#/testHelpers/chatModels";
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
// the chat. publish delivers stream events to the page.
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

	renderWithAuth(<Outlet context={outletContext} />, {
		path: "/agents",
		route: `/agents/${chat.id}`,
		children: [{ path: ":agentId", element: <AgentChatPage /> }],
	});

	const composer = () => screen.getByRole("textbox", { name: "Chat message" });
	// Lexical does not handle userEvent keystrokes in jsdom; a paste goes
	// through its input path.
	const type = async (text: string) => {
		await user.click(
			await screen.findByRole("textbox", { name: "Chat message" }),
		);
		await user.paste(text);
	};
	// The tests render one queued row, so its actions have unique names.
	const rowAction = async (action: string) => {
		await user.click(await screen.findByRole("button", { name: action }));
	};
	const publish = (event: TypesGen.ChatStreamEvent) =>
		act(() =>
			server?.publishMessage(
				new MessageEvent("message", { data: JSON.stringify([event]) }),
			),
		);
	return {
		user,
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
		vi.spyOn(apiModule, "watchChat").mockImplementation(
			() =>
				new OneWayWebSocket({
					apiRoute: `/api/experimental/chats/${mockProjectChat.id}/stream`,
					websocketInit: (url, protocol) =>
						createMockWebSocket(url, protocol)[0],
				}),
		);
		vi.spyOn(API.experimental, "getChat").mockResolvedValue(mockProjectChat);
		vi.spyOn(API.experimental, "getChatMessages").mockResolvedValue({
			messages: [],
			queued_messages: [],
			has_more: false,
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
		vi.spyOn(API.experimental, "getMCPServerConfigs").mockResolvedValue([]);
		const getChatProjectSkills = vi
			.spyOn(API.experimental, "getChatProjectSkills")
			.mockResolvedValue([{ ...MockSkill, name: "deploy", action: true }]);
		const createChatMessage = vi
			.spyOn(API.experimental, "createChatMessage")
			.mockRejectedValue(
				mockApiError({ message: 'Skill "deploy" is not available.' }),
			);
		const user = userEvent.setup();

		const { queryClient } = renderWithAuth(<Outlet context={outletContext} />, {
			path: "/agents",
			route: `/agents/${mockProjectChat.id}`,
			children: [{ path: ":agentId", element: <AgentChatPage /> }],
		});

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

	describe("queued message editing", () => {
		const row = {
			...MockChatQueuedMessage,
			id: 7,
			chat_id: MockChat.id,
			content: [{ type: "text" as const, text: "queued text" }],
		};

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
				await page.rowAction("Edit");
				await waitFor(() =>
					expect(page.composer().textContent).toBe("queued text"),
				);
				await page.type(" edit");

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
			},
		);
	});
});
