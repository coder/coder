import { renderHook, waitFor } from "@testing-library/react";
import type { WorkspaceAgentLog } from "api/typesGenerated";
import { act } from "react";
import { MockWorkspaceAgent } from "testHelpers/entities";
import {
	type MockWebSocketPublisher,
	createMockWebSocket,
} from "testHelpers/websockets";
import { OneWayWebSocket } from "utils/OneWayWebSocket";
import { createUseAgentLogs } from "./useAgentLogs";

const millisecondsInOneMinute = 60_000;

function generateMockLogs(
	logCount: number,
	baseDate = new Date(),
): readonly WorkspaceAgentLog[] {
	return Array.from({ length: logCount }, (_, i) => {
		// Make sure that the logs generated each have unique timestamps, so
		// that we can test whether the hook is sorting them properly as it's
		// receiving them over time
		const logDate = new Date(baseDate.getTime() + i * millisecondsInOneMinute);
		return {
			id: i,
			created_at: logDate.toISOString(),
			level: "info",
			output: `Log ${i}`,
			source_id: "",
		};
	});
}

// A mutable object holding the most recent mock WebSocket publisher. The inner
// value will change as the hook opens/closes new connections
type PublisherResult = { current: MockWebSocketPublisher };

type MountHookResult = Readonly<{
	rerender: (props: { agentId: string; enabled: boolean }) => void;
	publisherResult: PublisherResult;

	// Note: the `current` property is only "halfway" readonly; the value is
	// readonly, but the key is still mutable
	hookResult: { current: readonly WorkspaceAgentLog[] };
}>;

function mountHook(initialAgentId: string): MountHookResult {
	// Have to cheat the types a little bit to avoid a chicken-and-the-egg
	// scenario. publisherResult will be initialized with an undefined current
	// value, but it'll be guaranteed not to be undefined by the time mountHook
	// returns its value.
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
		({ agentId, enabled }) => useAgentLogs(agentId, enabled),
		{ initialProps: { agentId: initialAgentId, enabled: true } },
	);

	return {
		rerender,
		hookResult: result,
		publisherResult: publisherResult as PublisherResult,
	};
}

describe("useAgentLogs", () => {
	it("Automatically sorts logs that are received out of order", async () => {
		const { hookResult, publisherResult } = mountHook(MockWorkspaceAgent.id);
		const logs = generateMockLogs(10, new Date("september 9, 1999"));
		const reversed = logs.toReversed();

		for (const log of reversed) {
			act(() => {
				publisherResult.current.publishMessage(
					new MessageEvent<string>("message", {
						data: JSON.stringify([log]),
					}),
				);
			});
		}
		await waitFor(() => expect(hookResult.current).toEqual(logs));
	});

	it("Automatically closes the socket connection when the hook is disabled", async () => {
		const { publisherResult, rerender } = mountHook(MockWorkspaceAgent.id);
		expect(publisherResult.current.isConnectionOpen()).toBe(true);
		rerender({ agentId: MockWorkspaceAgent.id, enabled: false });
		await waitFor(() => {
			expect(publisherResult.current.isConnectionOpen()).toBe(false);
		});
	});

	it("Automatically closes the old connection when the agent ID changes", () => {
		const { publisherResult, rerender } = mountHook(MockWorkspaceAgent.id);
		const publisher1 = publisherResult.current;
		expect(publisher1.isConnectionOpen()).toBe(true);

		const newAgentId = `${MockWorkspaceAgent.id}-2`;
		rerender({ agentId: newAgentId, enabled: true });

		const publisher2 = publisherResult.current;
		expect(publisher1.isConnectionOpen()).toBe(false);
		expect(publisher2.isConnectionOpen()).toBe(true);
	});

	it("Clears logs when hook becomes disabled (protection to avoid duplicate logs when hook goes back to being re-enabled)", async () => {
		const { hookResult, publisherResult, rerender } = mountHook(
			MockWorkspaceAgent.id,
		);

		// Send initial logs so that we have something to clear out later
		const initialLogs = generateMockLogs(3, new Date("april 5, 1997"));
		const initialEvent = new MessageEvent<string>("message", {
			data: JSON.stringify(initialLogs),
		});
		act(() => publisherResult.current.publishMessage(initialEvent));
		await waitFor(() => expect(hookResult.current).toEqual(initialLogs));

		// Disable the hook (and have the hook close the connection behind the
		// scenes)
		rerender({ agentId: MockWorkspaceAgent.id, enabled: false });
		await waitFor(() => expect(hookResult.current).toHaveLength(0));

		// Re-enable the hook (creating an entirely new connection), and send
		// new logs
		rerender({ agentId: MockWorkspaceAgent.id, enabled: true });
		const newLogs = generateMockLogs(3, new Date("october 3, 2005"));
		const newEvent = new MessageEvent<string>("message", {
			data: JSON.stringify(newLogs),
		});
		act(() => publisherResult.current.publishMessage(newEvent));
		await waitFor(() => expect(hookResult.current).toEqual(newLogs));
	});
});
