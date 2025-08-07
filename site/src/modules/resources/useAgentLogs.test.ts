import { renderHook, waitFor } from "@testing-library/react";
import type { WorkspaceAgentLog } from "api/typesGenerated";
import { act } from "react";
import { MockWorkspaceAgent } from "testHelpers/entities";
import {
	type MockWebSocketServer,
	createMockWebSocket,
} from "testHelpers/websockets";
import { OneWayWebSocket } from "utils/OneWayWebSocket";
import {
	type CreateUseAgentLogsOptions,
	createUseAgentLogs,
} from "./useAgentLogs"

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

// A mutable object holding the most recent mock WebSocket server that was
// created when initializing a mock WebSocket. Inner value will be undefined if
// the hook is disabled on mount, but will always be defined otherwise
type ServerResult = { current: MockWebSocketServer | undefined };

type MountHookOptions = Readonly<{
	initialAgentId: string;
	enabled?: boolean;
	onError?: CreateUseAgentLogsOptions["onError"];
}>;

type MountHookResult = Readonly<{
	rerender: (props: { agentId: string; enabled: boolean }) => void;
	serverResult: ServerResult;

	// Note: the `current` property is only "halfway" readonly; the value is
	// readonly, but the key is still mutable
	hookResult: { current: readonly WorkspaceAgentLog[] };
}>;

function mountHook(options: MountHookOptions): MountHookResult {
	const { initialAgentId, enabled = true, onError = jest.fn() } = options;

	const serverResult: ServerResult = { current: undefined };
	const useAgentLogs = createUseAgentLogs({
		onError,
		createSocket: (agentId, params) => {
			return new OneWayWebSocket({
				apiRoute: `/api/v2/workspaceagents/${agentId}/logs`,
				searchParams: new URLSearchParams({
					follow: "true",
					after: params?.after?.toString() || "0",
				}),
				websocketInit: (url) => {
					const [mockSocket, mockServer] = createMockWebSocket(url);
					serverResult.current = mockServer;
					return mockSocket;
				},
			});
		},
	});

	const { result: hookResult, rerender } = renderHook(
		({ agentId, enabled }) => useAgentLogs(agentId, enabled),
		{ initialProps: { agentId: initialAgentId, enabled: enabled } },
	);

	return { rerender, serverResult, hookResult };
}

describe("useAgentLogs", () => {
	it("Automatically sorts logs that are received out of order", async () => {
		const { hookResult, serverResult } = mountHook({
			initialAgentId: MockWorkspaceAgent.id,
		});

		const logs = generateMockLogs(10, new Date("september 9, 1999"));
		const reversed = logs.toReversed();

		for (const log of reversed) {
			act(() => {
				serverResult.current?.publishMessage(
					new MessageEvent("message", { data: JSON.stringify([log]) }),
				);
			});
		}
		await waitFor(() => expect(hookResult.current).toEqual(logs));
	});

	it("Never creates a connection if hook is disabled on mount", () => {
		const { serverResult } = mountHook({
			initialAgentId: MockWorkspaceAgent.id,
			enabled: false,
		});

		expect(serverResult.current).toBe(undefined);
	});

	it("Automatically closes the socket connection when the hook is disabled", async () => {
		const { serverResult, rerender } = mountHook({
			initialAgentId: MockWorkspaceAgent.id,
		});

		expect(serverResult.current?.isConnectionOpen).toBe(true);
		rerender({ agentId: MockWorkspaceAgent.id, enabled: false });
		await waitFor(() => {
			expect(serverResult.current?.isConnectionOpen).toBe(false);
		});
	});

	it("Automatically closes the old connection when the agent ID changes", () => {
		const { serverResult, rerender } = mountHook({
			initialAgentId: MockWorkspaceAgent.id,
		});

		const serverConn1 = serverResult.current;
		expect(serverConn1?.isConnectionOpen).toBe(true);

		rerender({
			enabled: true,
			agentId: `${MockWorkspaceAgent.id}-new-value`,
		});

		const serverConn2 = serverResult.current;
		expect(serverConn1).not.toBe(serverConn2);
		expect(serverConn1?.isConnectionOpen).toBe(false);
		expect(serverConn2?.isConnectionOpen).toBe(true);
	});

	it("Calls error callback when error is received (but only while hook is enabled)", async () => {
		const onError = jest.fn();
		const { serverResult, rerender } = mountHook({
			initialAgentId: MockWorkspaceAgent.id,
			// Start off disabled so that we can check that the callback is
			// never called when there is no connection
			enabled: false,
			onError,
		});

		const errorEvent = new Event("error");
		act(() => serverResult.current?.publishError(errorEvent));
		expect(onError).not.toHaveBeenCalled();

		rerender({ agentId: MockWorkspaceAgent.id, enabled: true });
		act(() => serverResult.current?.publishError(errorEvent));
		expect(onError).toHaveBeenCalledTimes(1);
	});

	it("Clears logs when hook becomes disabled (protection to avoid duplicate logs when hook goes back to being re-enabled)", async () => {
		const { hookResult, serverResult, rerender } = mountHook({
			initialAgentId: MockWorkspaceAgent.id,
		});

		// Send initial logs so that we have something to clear out later
		const initialLogs = generateMockLogs(3, new Date("april 5, 1997"));
		const initialEvent = new MessageEvent("message", {
			data: JSON.stringify(initialLogs),
		});
		act(() => serverResult.current?.publishMessage(initialEvent));
		await waitFor(() => expect(hookResult.current).toEqual(initialLogs));

		// Need to do the following steps multiple times to make sure that we
		// don't break anything after the first disable
		const mockDates: readonly string[] = ["october 3, 2005", "august 1, 2025"];
		for (const md of mockDates) {
			// Disable the hook to clear current logs
			rerender({ agentId: MockWorkspaceAgent.id, enabled: false });
			await waitFor(() => expect(hookResult.current).toHaveLength(0));

			// Re-enable the hook and send new logs
			rerender({ agentId: MockWorkspaceAgent.id, enabled: true });
			const newLogs = generateMockLogs(3, new Date(md));
			const newEvent = new MessageEvent("message", {
				data: JSON.stringify(newLogs),
			});
			act(() => serverResult.current?.publishMessage(newEvent));
			await waitFor(() => expect(hookResult.current).toEqual(newLogs));
		}
	});
});
