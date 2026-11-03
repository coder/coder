import { act, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http } from "msw";
import { Outlet } from "react-router";
import { afterEach, beforeAll, describe, expect, it, vi } from "vitest";
import * as apiModule from "#/api/api";
import { API } from "#/api/api";
import type * as TypesGen from "#/api/typesGenerated";
import { MockChat, MockChatQueuedMessage } from "#/testHelpers/chatEntities";
import {
	MockChatModel,
	MockChatModelProviderDescriptor,
} from "#/testHelpers/chatModels";
import { createDeferred } from "#/testHelpers/deferred";
import { MockUserPreferenceSettings } from "#/testHelpers/entities";
import { renderWithAuth } from "#/testHelpers/renderHelpers";
import { server } from "#/testHelpers/server";
import {
	createMockWebSocket,
	type MockWebSocketServer,
} from "#/testHelpers/websockets";
import { OneWayWebSocket } from "#/utils/OneWayWebSocket";
import type { AgentsPageOutletContext } from "../AgentsPageLayout";
import AgentChatPage from "./AgentChatPage";

// Response bodies copied from coderd/exp_chats.go. promoteChatQueuedMessage
// and deleteChatQueuedMessage return 404 when the chat still has a queue but
// not this row (chatstate.ErrQueuedMessageNotFound), and 409 when the chat's
// state no longer allows the transition at all. After the worker delivers the
// last queued row the chat is running with an empty queue (chatstate R0),
// which allows neither SteerQueuedMessage nor DeleteQueuedMessage, so the
// server answers 409 with the TransitionError text as detail.
const proofCPromoteGone = {
	status: 404,
	body: { message: "Queued message not found." },
};
const proofCPromoteDelivered = {
	status: 409,
	body: {
		message: "Chat has no queued messages to promote.",
		detail:
			"chat state transition SteerQueuedMessage not allowed from state R0",
	},
};
const proofCDeleteGone = {
	status: 404,
	body: { message: "Queued message not found." },
};
const proofCDeleteDelivered = {
	status: 409,
	body: {
		message: "Chat has no queued messages to delete.",
		detail:
			"chat state transition DeleteQueuedMessage not allowed from state R0",
	},
};

type ProofCResponse = { status: number; body: TypesGen.Response };

const proofCRow: TypesGen.ChatQueuedMessage = {
	...MockChatQueuedMessage,
	id: 7,
	chat_id: MockChat.id,
	content: [{ type: "text", text: "queued text" }],
	busy_behavior: "queue",
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

// Renders the page for a running chat that has one queued row. The server
// side is modeled by serverQueue: the page's message fetches read it, and
// deliverRow empties it and publishes the queue_update the worker publishes
// when it delivers the row. The promote and delete endpoints are served by
// MSW through the real API client and hold their response until the test
// releases it, so the test orders the response against the queue_update.
const proofCRenderRunningChatWithRow = ({
	endpoint,
	response,
}: {
	endpoint: "promote" | "delete";
	response: ProofCResponse;
}) => {
	const chat: TypesGen.Chat = { ...MockChat, status: "running" };
	let socketServer: MockWebSocketServer | undefined;
	let serverQueue: TypesGen.ChatQueuedMessage[] = [proofCRow];
	vi.spyOn(apiModule, "watchChat").mockImplementation(
		() =>
			new OneWayWebSocket({
				apiRoute: `/api/experimental/chats/${chat.id}/stream`,
				websocketInit: (url, protocol) => {
					const [socket, mockServer] = createMockWebSocket(url, protocol);
					socketServer = mockServer;
					return socket;
				},
			}),
	);
	vi.spyOn(API.experimental, "getChat").mockResolvedValue(chat);
	vi.spyOn(API.experimental, "getChatMessages").mockImplementation(
		async () => ({
			messages: [],
			queued_messages: [...serverQueue],
			has_more: false,
		}),
	);
	vi.spyOn(API.experimental, "getChatModels").mockResolvedValue({
		models: [
			{
				...MockChatModel,
				id: chat.last_model_config_id,
				organization_id: chat.organization_id,
			},
		],
		providers: [MockChatModelProviderDescriptor],
		unsupported_providers: [],
	});
	vi.spyOn(API, "getUserPreferenceSettings").mockResolvedValue(
		MockUserPreferenceSettings,
	);
	vi.spyOn(API.experimental, "getMCPServerConfigs").mockResolvedValue([]);

	const requestReceived = createDeferred<undefined>();
	const releaseResponse = createDeferred<undefined>();
	const path =
		endpoint === "promote"
			? `/api/v2/chats/${chat.id}/queue/${proofCRow.id}/promote`
			: `/api/v2/chats/${chat.id}/queue/${proofCRow.id}`;
	const respond = async () => {
		requestReceived.resolve(undefined);
		await releaseResponse.promise;
		return HttpResponse.json(response.body, { status: response.status });
	};
	server.use(
		endpoint === "promote"
			? http.post(path, respond)
			: http.delete(path, respond),
	);

	const outletContext: Partial<AgentsPageOutletContext> = {
		chatErrorReasons: {},
		setChatErrorReason: vi.fn(),
		clearChatErrorReason: vi.fn(),
		onChatReady: vi.fn(),
	};
	const user = userEvent.setup();
	const { container, queryClient } = renderWithAuth(
		<Outlet context={outletContext} />,
		{
			path: "/agents",
			route: `/agents/${chat.id}`,
			children: [{ path: ":agentId", element: <AgentChatPage /> }],
		},
	);
	const view = within(container);

	const deliverRow = () => {
		serverQueue = [];
		return act(() =>
			socketServer?.publishMessage(
				new MessageEvent("message", {
					data: JSON.stringify([
						{
							type: "queue_update",
							chat_id: chat.id,
							queued_messages: [],
						} satisfies TypesGen.ChatStreamEvent,
					]),
				}),
			),
		);
	};
	// Releases the held response and waits until the page has handled it.
	const settleResponse = async () => {
		releaseResponse.resolve(undefined);
		await waitFor(() => expect(queryClient.isMutating()).toBe(0));
		// The page's catch handler runs after react-query settles the mutation.
		await act(async () => {});
	};
	return {
		user,
		view,
		outletContext,
		requestReceived: requestReceived.promise,
		deliverRow,
		settleResponse,
	};
};

describe("TestProof steer stack: queued row delivered by the worker", () => {
	// The worker delivers the row after the user clicked Send now and before
	// the server handled the promote. The server publishes queue_update [] at
	// delivery; it can reach the page before or after the promote response.
	it.each([
		["404, queue_update first", proofCPromoteGone, true],
		["404, response first", proofCPromoteGone, false],
		["409, queue_update first", proofCPromoteDelivered, true],
		["409, response first", proofCPromoteDelivered, false],
	] as const)(
		"TestProofSendNowOnDeliveredRowShowsNoError (%s)",
		async (_name, response, queueUpdateFirst) => {
			const page = proofCRenderRunningChatWithRow({
				endpoint: "promote",
				response,
			});
			await page.user.click(
				await page.view.findByRole("button", { name: "Send now" }),
			);
			await page.requestReceived;

			if (queueUpdateFirst) {
				await page.deliverRow();
			}
			await page.settleResponse();

			// The row is gone and no error is surfaced, as for a missing row.
			expect(
				screen.queryByText(response.body.message ?? ""),
			).not.toBeInTheDocument();
			expect(page.outletContext.setChatErrorReason).not.toHaveBeenCalled();
			expect(
				page.view.queryByRole("button", { name: "Send now" }),
			).not.toBeInTheDocument();

			if (!queueUpdateFirst) {
				await page.deliverRow();
				expect(page.outletContext.setChatErrorReason).not.toHaveBeenCalled();
			}
		},
	);

	// Remove from queue on a row the worker delivered first. The page gives
	// no error for a failed delete; the visible result is the row coming back
	// until the delivery's queue_update arrives.
	it.each([
		["404", proofCDeleteGone],
		["409", proofCDeleteDelivered],
	] as const)(
		"TestProofRemoveDeliveredRowDoesNotReappear (%s, response first)",
		async (_name, response) => {
			const page = proofCRenderRunningChatWithRow({
				endpoint: "delete",
				response,
			});
			await page.user.click(
				await page.view.findByRole("button", { name: "Remove from queue" }),
			);
			await page.requestReceived;
			await page.settleResponse();

			expect(
				page.view.queryByRole("button", { name: "Remove from queue" }),
			).not.toBeInTheDocument();

			await page.deliverRow();
			expect(
				page.view.queryByRole("button", { name: "Remove from queue" }),
			).not.toBeInTheDocument();
		},
	);
});
