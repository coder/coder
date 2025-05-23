import { renderHook, waitFor } from "@testing-library/react";
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

// A mutable object holding the most recent mock WebSocket publisher. The inner
// value will change as the hook opens/closes new connections
type PublisherResult = {
	current: MockWebSocketPublisher;
};

type MountHookResult = Readonly<{
	// Note: the value of `current` should be readonly, but the `current`
	// property itself should be mutable
	hookResult: {
		current: readonly WorkspaceAgentLog[];
	};
	rerender: (props: { enabled: boolean }) => void;
	publisherResult: PublisherResult;
}>;

function mountHook(): MountHookResult {
	// Have to cheat the types a little bit to avoid a chicken-and-the-egg
	// scenario. publisherResult will be initialized with an undefined current
	// value, but it'll be guaranteed not to be undefined by the time this
	// function returns.
	const publisherResult: Partial<PublisherResult> = { current: undefined };
	const useAgentLogs = createUseAgentLogs((agentId, params) => {
		return new OneWayWebSocket({
			apiRoute: `/api/v2/workspaceagents/${agentId}/logs`,
			searchParams: new URLSearchParams({
				follow: "true",
				after: params?.after?.toString() || "0",
			}),
			websocketInit: (url) => {
				const [mockSocket, mockPublisher] = createMockWebSocket(url);
				publisherResult.current = mockPublisher;
				return mockSocket;
			},
		});
	});

	const { result, rerender } = renderHook(
		({ enabled }) => useAgentLogs(MockWorkspaceAgent, enabled),
		{ initialProps: { enabled: true } },
	);

	return {
		rerender,
		hookResult: result,
		publisherResult: publisherResult as PublisherResult,
	};
}

describe("useAgentLogs", () => {
	it("clears logs when hook becomes disabled (protection to avoid duplicate logs when hook goes back to being re-enabled)", async () => {
		const { hookResult, publisherResult, rerender } = mountHook();

		// Verify that logs can be received after mount
		const initialLogs = generateMockLogs(3);
		const initialEvent = new MessageEvent<string>("message", {
			data: JSON.stringify(initialLogs),
		});
		publisherResult.current.publishMessage(initialEvent);
		await waitFor(() => {
			// Using expect.arrayContaining to account for the fact that we're
			// not guaranteed to receive WebSocket events in order
			expect(hookResult.current).toEqual(expect.arrayContaining(initialLogs));
		});

		// Disable the hook (and have the hook close the connection behind the
		// scenes)
		rerender({ enabled: false });
		await waitFor(() => expect(hookResult.current).toHaveLength(0));

		// Re-enable the hook (creating an entirely new connection), and send
		// new logs
		rerender({ enabled: true });
		const newLogs = generateMockLogs(3);
		const newEvent = new MessageEvent<string>("message", {
			data: JSON.stringify(newLogs),
		});
		publisherResult.current.publishMessage(newEvent);
		await waitFor(() => {
			expect(hookResult.current).toEqual(expect.arrayContaining(newLogs));
		});
	});
});
