import { renderHook, waitFor } from "@testing-library/react";
import type { WorkspaceAgentLog } from "api/typesGenerated";
import WS from "jest-websocket-mock";
import { MockWorkspaceAgent } from "testHelpers/entities";
import { useAgentLogs } from "./useAgentLogs";

/**
 * TODO: WS does not support multiple tests running at once in isolation so we
 * have one single test that test the most common scenario.
 * Issue: https://github.com/romgain/jest-websocket-mock/issues/172
 */

describe.skip("useAgentLogs", () => {
	afterEach(() => {
		WS.clean();
	});

	it("clear logs when disabled to avoid duplicates", async () => {
		const server = new WS(
			`ws://localhost/api/v2/workspaceagents/${
				MockWorkspaceAgent.id
			}/logs?follow&after=0`,
		);
		const { result, rerender } = renderHook(
			({ enabled }) => useAgentLogs(MockWorkspaceAgent, enabled),
			{ initialProps: { enabled: true } },
		);
		await server.connected;

		// Send 3 logs
		server.send(JSON.stringify(generateLogs(3)));
		await waitFor(() => {
			expect(result.current).toHaveLength(3);
		});

		// Disable the hook
		rerender({ enabled: false });
		await waitFor(() => {
			expect(result.current).toHaveLength(0);
		});

		// Enable the hook again
		rerender({ enabled: true });
		await server.connected;
		server.send(JSON.stringify(generateLogs(3)));
		await waitFor(() => {
			expect(result.current).toHaveLength(3);
		});
	});
});

function generateLogs(count: number): WorkspaceAgentLog[] {
	return Array.from({ length: count }, (_, i) => ({
		id: i,
		created_at: new Date().toISOString(),
		level: "info",
		output: `Log ${i}`,
		source_id: "",
	}));
}
