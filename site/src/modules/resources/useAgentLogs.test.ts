import {
	renderHook,
	type RenderHookResult,
	waitFor,
} from "@testing-library/react";
import type { WorkspaceAgentLog } from "api/typesGenerated";
import { MockWorkspaceAgent } from "testHelpers/entities";
import { createUseAgentLogs } from "./useAgentLogs";
import {
	createMockWebSocket,
	type MockWebSocketPublisher,
} from "testHelpers/websockets";
import { OneWayWebSocket } from "utils/OneWayWebSocket";

function generateMockLogs(count: number): WorkspaceAgentLog[] {
	return Array.from({ length: count }, (_, i) => ({
		id: i,
		created_at: new Date().toISOString(),
		level: "info",
		output: `Log ${i}`,
		source_id: "",
	}));
}

type MountHookResult = Readonly<
	RenderHookResult<readonly WorkspaceAgentLog[], { enabled: boolean }> & {
		publisher: MockWebSocketPublisher;
	}
>;

function mountHook(): MountHookResult {
	let publisher!: MockWebSocketPublisher;
	const useAgentLogs = createUseAgentLogs((agentId, params) => {
		return new OneWayWebSocket({
			apiRoute: `/api/v2/workspaceagents/${agentId}/logs`,
			searchParams: new URLSearchParams({
				follow: "true",
				after: params?.after?.toString() || "0",
			}),
			websocketInit: (url) => {
				const [mockSocket, mockPub] = createMockWebSocket(url);
				publisher = mockPub;
				return mockSocket;
			},
		});
	});

	const { result, rerender, unmount } = renderHook(
		({ enabled }) => useAgentLogs(MockWorkspaceAgent, enabled),
		{ initialProps: { enabled: true } },
	);

	return { result, rerender, unmount, publisher };
}

describe("useAgentLogs", () => {
	it("clears logs when hook becomes disabled (protection to avoid duplicate logs when hook goes back to being re-enabled)", async () => {
		const { result, rerender, publisher } = mountHook();

		// Verify that logs can be received after mount
		const initialLogs = generateMockLogs(3);
		const initialEvent = new MessageEvent<string>("message", {
			data: JSON.stringify(initialLogs),
		});
		publisher.publishMessage(initialEvent);
		await waitFor(() => {
			// Using expect.arrayContaining to account for the fact that we're
			// not guaranteed to receive WebSocket events in order
			expect(result.current).toEqual(expect.arrayContaining(initialLogs));
		});

		// Disable the hook (and have the hook close the connection behind the
		// scenes)
		rerender({ enabled: false });
		await waitFor(() => expect(result.current).toHaveLength(0));

		// Re-enable the hook (creating an entirely new connection), and send
		// new logs
		rerender({ enabled: true });
		const newLogs = generateMockLogs(3);
		const newEvent = new MessageEvent<string>("message", {
			data: JSON.stringify(newLogs),
		});
		publisher.publishMessage(newEvent);
		await waitFor(() => {
			expect(result.current).toEqual(expect.arrayContaining(newLogs));
		});
	});
});
