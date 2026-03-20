import { MockWorkspaceAgent } from "testHelpers/entities";
import {
	createMockWebSocket,
	type MockWebSocketServer,
} from "testHelpers/websockets";
import { renderHook, waitFor } from "@testing-library/react";
import * as apiModule from "api/api";
import type { WorkspaceAgentLog } from "api/typesGenerated";
import { act } from "react";
import { toast } from "sonner";
import {
	type OneWayMessageEvent,
	OneWayWebSocket,
} from "utils/OneWayWebSocket";
import { MAX_LOGS, useAgentLogs } from "./useAgentLogs";

const millisecondsInOneMinute = 60_000;

function generateMockLogs(
	logCount: number,
	baseDate = new Date("April 1, 1970"),
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
	maxLogs?: number;
}>;

type MountHookResult = Readonly<{
	serverResult: ServerResult;
	rerender: (props: {
		agentId: string;
		enabled: boolean;
		maxLogs?: number;
	}) => void;
	toastError: jest.SpyInstance;

	// Note: the `current` property is only "halfway" readonly; the value is
	// readonly, but the key is still mutable
	hookResult: { current: readonly WorkspaceAgentLog[] };
}>;

type ManualAgentLogSocket = Readonly<{
	socket: OneWayWebSocket<WorkspaceAgentLog[]>;
	publishMessage: (logs: readonly WorkspaceAgentLog[]) => void;
	publishError: (event: Event) => void;
	getListenerCount: (eventType: "error" | "message") => number;
	close: jest.Mock;
}>;

function mountHook(options: MountHookOptions): MountHookResult {
	const { initialAgentId, enabled = true, maxLogs } = options;
	const serverResult: ServerResult = { current: undefined };

	jest
		.spyOn(apiModule, "watchWorkspaceAgentLogs")
		.mockImplementation((agentId, params) => {
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
		});

	void jest.spyOn(console, "error").mockImplementation(() => {});
	const toastError = jest.spyOn(toast, "error");

	const { result: hookResult, rerender } = renderHook(
		(props) => useAgentLogs(props),
		{ initialProps: { enabled, agentId: initialAgentId, maxLogs } },
	);
	const rerenderHook = (props: {
		agentId: string;
		enabled: boolean;
		maxLogs?: number;
	}) => {
		rerender({ ...props, maxLogs: props.maxLogs });
	};

	return { rerender: rerenderHook, serverResult, hookResult, toastError };
}

function createManualAgentLogSocket(): ManualAgentLogSocket {
	const listeners = {
		error: new Set<(event: Event) => void>(),
		message: new Set<
			(event: OneWayMessageEvent<readonly WorkspaceAgentLog[]>) => void
		>(),
	};
	const close = jest.fn();

	const socket = {
		addEventListener: jest.fn(
			(
				eventType: "error" | "message",
				callback:
					| ((event: Event) => void)
					| ((event: OneWayMessageEvent<readonly WorkspaceAgentLog[]>) => void),
			) => {
				if (eventType === "message") {
					listeners.message.add(
						callback as (
							event: OneWayMessageEvent<readonly WorkspaceAgentLog[]>,
						) => void,
					);
					return;
				}
				listeners.error.add(callback as (event: Event) => void);
			},
		),
		removeEventListener: jest.fn(
			(
				eventType: "error" | "message",
				callback:
					| ((event: Event) => void)
					| ((event: OneWayMessageEvent<readonly WorkspaceAgentLog[]>) => void),
			) => {
				if (eventType === "message") {
					listeners.message.delete(
						callback as (
							event: OneWayMessageEvent<readonly WorkspaceAgentLog[]>,
						) => void,
					);
					return;
				}
				listeners.error.delete(callback as (event: Event) => void);
			},
		),
		close,
	} as unknown as OneWayWebSocket<WorkspaceAgentLog[]>;

	return {
		socket,
		publishMessage: (logs) => {
			const event: OneWayMessageEvent<readonly WorkspaceAgentLog[]> = {
				sourceEvent: new MessageEvent("message", {
					data: JSON.stringify(logs),
				}),
				parseError: undefined,
				parsedMessage: logs,
			};
			for (const listener of [...listeners.message]) {
				listener(event);
			}
		},
		publishError: (event) => {
			for (const listener of [...listeners.error]) {
				listener(event);
			}
		},
		getListenerCount: (eventType) => listeners[eventType].size,
		close,
	};
}

afterEach(() => {
	jest.restoreAllMocks();
});

describe("useAgentLogs", () => {
	it("Automatically sorts logs that are received out of order", async () => {
		const { hookResult, serverResult } = mountHook({
			initialAgentId: MockWorkspaceAgent.id,
		});

		const logs = generateMockLogs(10, new Date("september 9, 1999"));
		const reversed = logs.toReversed();

		for (const log of reversed) {
			await act(async () => {
				serverResult.current?.publishMessage(
					new MessageEvent("message", { data: JSON.stringify([log]) }),
				);
			});
		}
		await waitFor(() => expect(hookResult.current).toEqual(logs));
	});

	it("keeps the full log history by default", async () => {
		const { hookResult, serverResult } = mountHook({
			initialAgentId: MockWorkspaceAgent.id,
		});

		const logs = generateMockLogs(MAX_LOGS + 25, new Date("April 9, 2001"));

		await act(async () => {
			serverResult.current?.publishMessage(
				new MessageEvent("message", { data: JSON.stringify(logs) }),
			);
		});

		await waitFor(() => expect(hookResult.current).toEqual(logs));
	});

	it("retains only the newest bounded number of logs when requested", async () => {
		const { hookResult, serverResult } = mountHook({
			initialAgentId: MockWorkspaceAgent.id,
			maxLogs: MAX_LOGS,
		});

		const logs = generateMockLogs(MAX_LOGS + 25, new Date("April 10, 2001"));
		const retainedLogs = logs.slice(-MAX_LOGS);

		await act(async () => {
			serverResult.current?.publishMessage(
				new MessageEvent("message", { data: JSON.stringify(logs) }),
			);
		});

		await waitFor(() => expect(hookResult.current).toEqual(retainedLogs));
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

	it("Stops processing log updates after cleanup runs", async () => {
		const manualSocket = createManualAgentLogSocket();
		jest
			.spyOn(apiModule, "watchWorkspaceAgentLogs")
			.mockReturnValue(manualSocket.socket);

		const { result, rerender } = renderHook(
			(props: { agentId: string; enabled: boolean }) => useAgentLogs(props),
			{
				initialProps: {
					agentId: MockWorkspaceAgent.id,
					enabled: true,
				},
			},
		);

		const initialLogs = generateMockLogs(2, new Date("June 1, 2002"));
		await act(async () => {
			manualSocket.publishMessage(initialLogs);
		});
		await waitFor(() => expect(result.current).toEqual(initialLogs));

		rerender({ agentId: MockWorkspaceAgent.id, enabled: false });
		await waitFor(() => expect(result.current).toHaveLength(0));
		expect(manualSocket.close).toHaveBeenCalledTimes(1);
		expect(manualSocket.getListenerCount("message")).toBe(0);
		expect(manualSocket.getListenerCount("error")).toBe(0);

		const lateLogs = generateMockLogs(1, new Date("June 2, 2002"));
		await act(async () => {
			manualSocket.publishMessage(lateLogs);
		});
		expect(result.current).toEqual([]);
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
		const { serverResult, rerender, toastError } = mountHook({
			initialAgentId: MockWorkspaceAgent.id,
			// Start off disabled so that we can check that the callback is
			// never called when there is no connection
			enabled: false,
		});

		const errorEvent = new Event("error");
		await act(async () => serverResult.current?.publishError(errorEvent));
		expect(toastError).not.toHaveBeenCalled();

		rerender({ agentId: MockWorkspaceAgent.id, enabled: true });
		await act(async () => serverResult.current?.publishError(errorEvent));
		expect(toastError).toHaveBeenCalledTimes(1);
	});

	// This is a protection to avoid duplicate logs when the hook goes back to
	// being re-enabled
	it("Clears logs when hook becomes disabled", async () => {
		const { hookResult, serverResult, rerender } = mountHook({
			initialAgentId: MockWorkspaceAgent.id,
		});

		// Send initial logs so that we have something to clear out later
		const initialLogs = generateMockLogs(3, new Date("april 5, 1997"));
		const initialEvent = new MessageEvent("message", {
			data: JSON.stringify(initialLogs),
		});
		await act(async () => serverResult.current?.publishMessage(initialEvent));
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
			await act(async () => serverResult.current?.publishMessage(newEvent));
			await waitFor(() => expect(hookResult.current).toEqual(newLogs));
		}
	});
});
