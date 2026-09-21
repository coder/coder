import { act, render, waitFor } from "@testing-library/react";
import { QueryClientProvider } from "react-query";
import { afterEach, expect, it, vi } from "vitest";
import { API } from "#/api/api";
import { acpSession, acpSessionPath } from "#/api/queries/acp";
import { MockACPSession } from "#/testHelpers/acp";
import { createTestQueryClient } from "#/testHelpers/renderHelpers";
import { createMockWebSocket } from "#/testHelpers/websockets";
import { useACPSession } from "./useACPSession";

const session = MockACPSession;
const path = acpSessionPath(
	session.parent_chat_id,
	session.workspace_agent_id,
	session.session_id,
);
function Consumer() {
	useACPSession(path);
	return null;
}
afterEach(() => {
	vi.unstubAllGlobals();
	vi.restoreAllMocks();
});

it("shares a stream and rejects stale snapshots without closing a surviving subscriber", async () => {
	const [socket, server] = createMockWebSocket(`ws://localhost${path}/stream`);
	const construct = vi.fn(function (this: WebSocket) {
		Object.assign(this, socket);
	});
	vi.stubGlobal("WebSocket", construct);
	vi.spyOn(API, "getACPSession").mockResolvedValue(session);
	const client = createTestQueryClient();
	const view = render(
		<QueryClientProvider client={client}>
			<Consumer />
			<Consumer />
		</QueryClientProvider>,
	);
	await waitFor(() =>
		expect(client.getQueryData(acpSession(path).queryKey)).toEqual(session),
	);
	expect(construct).toHaveBeenCalledOnce();
	act(() => {
		server.publishMessage(
			new MessageEvent("message", {
				data: JSON.stringify({ ...session, version: 5, status: "waiting" }),
			}),
		);
		server.publishMessage(
			new MessageEvent("message", { data: JSON.stringify(session) }),
		);
	});
	expect(client.getQueryData(acpSession(path).queryKey)).toMatchObject({
		version: 5,
		status: "waiting",
	});
	view.rerender(
		<QueryClientProvider client={client}>
			<Consumer />
		</QueryClientProvider>,
	);
	expect(server.isConnectionOpen).toBe(true);
	view.unmount();
	expect(server.isConnectionOpen).toBe(false);
});

it("replaces transcript history with an expired result", async () => {
	vi.spyOn(API, "getACPSession").mockResolvedValue(null);
	const client = createTestQueryClient();
	client.setQueryData(acpSession(path).queryKey, session);
	await client.fetchQuery(acpSession(path));
	expect(client.getQueryData(acpSession(path).queryKey)).toBeNull();
});
