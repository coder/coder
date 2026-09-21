import type * as MessageScrollerModule from "@shadcn/react/message-scroller";
import { act, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { createMemoryRouter } from "react-router";
import { afterEach, expect, it, vi } from "vitest";
import { API } from "#/api/api";
import { acpSessionPath } from "#/api/queries/acp";
import { MockACPSession } from "#/testHelpers/acp";
import { MockChat } from "#/testHelpers/chatEntities";
import { renderWithRouter } from "#/testHelpers/renderHelpers";
import { createMockWebSocket } from "#/testHelpers/websockets";
import ACPChatPage from "./ACPChatPage";

const { scrollToEnd } = vi.hoisted(() => ({ scrollToEnd: vi.fn() }));
vi.mock("@shadcn/react/message-scroller", async (importOriginal) => {
	const original = await importOriginal<typeof MessageScrollerModule>();
	return {
		...original,
		useMessageScroller: () => ({
			...original.useMessageScroller(),
			scrollToEnd,
		}),
	};
});

vi.mock("#/modules/dashboard/useDashboard", () => ({
	useDashboard: () => ({ organizations: [] }),
}));
afterEach(() => {
	vi.restoreAllMocks();
	vi.unstubAllGlobals();
});

it("sends and interrupts through ACP endpoints and scrolls after sending", async () => {
	const createRange = document.createRange.bind(document);
	vi.spyOn(document, "createRange").mockImplementation(() =>
		Object.assign(createRange(), {
			getBoundingClientRect: () => new DOMRect(0, 0, 1, 16),
		}),
	);
	const getRangeAt = Selection.prototype.getRangeAt;
	vi.spyOn(Selection.prototype, "getRangeAt").mockImplementation(function (
		this: Selection,
		index: number,
	) {
		return Object.assign(getRangeAt.call(this, index), {
			getBoundingClientRect: () => new DOMRect(0, 0, 1, 16),
		});
	});
	const session = MockACPSession;
	const path = acpSessionPath(
		session.parent_chat_id,
		session.workspace_agent_id,
		session.session_id,
	);
	const [socket, server] = createMockWebSocket(`ws://localhost${path}/stream`);
	vi.stubGlobal(
		"WebSocket",
		vi.fn(function (this: WebSocket) {
			Object.assign(this, socket);
		}),
	);
	vi.spyOn(API, "getACPSession").mockResolvedValue(session);
	vi.spyOn(API.experimental, "getChat").mockResolvedValue(MockChat);
	vi.spyOn(API.experimental, "getUserSkills").mockResolvedValue([]);
	const send = vi.spyOn(API, "sendACPMessage").mockResolvedValue(session);
	const interrupt = vi
		.spyOn(API, "interruptACPSession")
		.mockResolvedValue(session);
	const regularSend = vi.spyOn(API.experimental, "createChatMessage");
	const router = createMemoryRouter(
		[
			{
				path: "/agents/:agentId/acp/:workspaceAgentId/:sessionId",
				element: <ACPChatPage />,
			},
		],
		{
			initialEntries: [
				`/agents/${session.parent_chat_id}/acp/${session.workspace_agent_id}/${session.session_id}`,
			],
		},
	);
	renderWithRouter(router);
	await waitFor(() => expect(API.getACPSession).toHaveBeenCalledWith(path));
	act(() =>
		server.publishMessage(
			new MessageEvent("message", { data: JSON.stringify(session) }),
		),
	);
	const user = userEvent.setup();
	await user.click(screen.getByRole("textbox", { name: "Chat message" }));
	await user.paste("Check the next test.");
	await user.keyboard("{Enter}");
	await waitFor(() =>
		expect(send).toHaveBeenCalledWith(path, {
			message: "Check the next test.",
		}),
	);
	await waitFor(() =>
		expect(scrollToEnd).toHaveBeenCalledWith({ behavior: "smooth" }),
	);
	await user.click(screen.getByRole("button", { name: "Stop" }));
	await waitFor(() => expect(interrupt).toHaveBeenCalledWith(path));
	expect(regularSend).not.toHaveBeenCalled();
});
