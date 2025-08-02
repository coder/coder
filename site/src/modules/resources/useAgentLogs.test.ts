import { renderHook, waitFor } from "@testing-library/react";
import type { WorkspaceAgentLog } from "api/typesGenerated";
import { MockWorkspaceAgent } from "testHelpers/entities";
import {
	type MockWebSocketPublisher,
	createMockWebSocket,
} from "testHelpers/websockets";
import { OneWayWebSocket } from "utils/OneWayWebSocket";
import { createUseAgentLogs } from "./useAgentLogs";
import { act } from "react";

const millisecondsInOneMinute = 60_000;

function generateMockLogs(
	logCount: number,
	baseDate = new Date(),
): readonly WorkspaceAgentLog[] {
	return Array.from({ length: logCount }, (_, i) => {
		// Make sure that the logs generated each have unique timestamps, so
		// that we can test whether they're being sorted properly before being
		// returned by the hook
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
	it("Automatically sorts logs that are received out of order", async () => {
		const { hookResult, publisherResult } = mountHook();
		const logs = generateMockLogs(10, new Date("september 9, 1999"));

		for (const log of logs.toReversed()) {
			act(() => {
				publisherResult.current.publishMessage(
					new MessageEvent<string>("message", {
						data: JSON.stringify([log]),
					}),
				)
			})
		}
		await waitFor(() => expect(hookResult.current).toEqual(logs));
	});

	it("Automatically closes the socket connection when the hook is disabled", async () => {
		const { publisherResult, rerender } = mountHook();
		expect(publisherResult.current.isConnectionOpen()).toBe(true);
		rerender({ enabled: false });
		await waitFor(() => {
			expect(publisherResult.current.isConnectionOpen()).toBe(false);
		})
	});

	it("Clears logs when hook becomes disabled (protection to avoid duplicate logs when hook goes back to being re-enabled)", async () => {
		const { hookResult, publisherResult, rerender } = mountHook();

		// Send initial logs so that we have something to clear out later
		const initialLogs = generateMockLogs(3, new Date("april 5, 1997"));
		const initialEvent = new MessageEvent<string>("message", {
			data: JSON.stringify(initialLogs),
		});
		act(() => publisherResult.current.publishMessage(initialEvent));
		await waitFor(() => expect(hookResult.current).toEqual(initialLogs));

		// Disable the hook (and have the hook close the connection behind the
		// scenes)
		rerender({ enabled: false });
		await waitFor(() => expect(hookResult.current).toHaveLength(0));

		// Re-enable the hook (creating an entirely new connection), and send
		// new logs
		rerender({ enabled: true });
		const newLogs = generateMockLogs(3, new Date("october 3, 2005"));
		const newEvent = new MessageEvent<string>("message", {
			data: JSON.stringify(newLogs),
		});
		act(() => publisherResult.current.publishMessage(newEvent));
		await waitFor(() => expect(hookResult.current).toEqual(newLogs));
	});
});
