import { render } from "@testing-library/react";
import { QueryClientProvider } from "react-query";
import { afterEach, describe, expect, it, vi } from "vitest";
import * as apiModule from "#/api/api";
import { API } from "#/api/api";
import { workspaceByIdKey } from "#/api/queries/workspaces";
import { MockWorkspace, MockWorkspaceAgent } from "#/testHelpers/entities";
import { createTestQueryClient } from "#/testHelpers/renderHelpers";
import { createMockWebSocket } from "#/testHelpers/websockets";
import { OneWayWebSocket } from "#/utils/OneWayWebSocket";
import { ChatWorkspaceContext } from "../../../context/ChatWorkspaceContext";
import { Tool } from "./Tool";

afterEach(() => {
	vi.restoreAllMocks();
});

describe("Tool workspace lifecycle rows", () => {
	it.each([
		{ name: "start_workspace", agentLogs: true },
		{ name: "create_workspace", agentLogs: true },
		{ name: "stop_workspace", agentLogs: false },
	])(
		"$name streams build logs, agent logs: $agentLogs",
		({ name, agentLogs }) => {
			const watchBuildLogs = vi
				.spyOn(apiModule, "watchBuildLogsByBuildId")
				.mockImplementation(() => createMockWebSocket("ws://test")[0]);
			const watchAgentLogs = vi
				.spyOn(apiModule, "watchWorkspaceAgentLogs")
				.mockImplementation(
					(agentId) =>
						new OneWayWebSocket({
							apiRoute: `/api/v2/workspaceagents/${agentId}/logs`,
							websocketInit: (url, protocol) =>
								createMockWebSocket(url, protocol)[0],
						}),
				);
			vi.spyOn(API, "getWorkspace").mockResolvedValue(MockWorkspace);
			const queryClient = createTestQueryClient();
			queryClient.setQueryData(
				workspaceByIdKey(MockWorkspace.id),
				MockWorkspace,
			);

			render(
				<QueryClientProvider client={queryClient}>
					<ChatWorkspaceContext
						value={{
							workspaceId: MockWorkspace.id,
							buildId: MockWorkspace.latest_build.id,
							agentId: MockWorkspaceAgent.id,
						}}
					>
						<Tool name={name} status="running" />
					</ChatWorkspaceContext>
				</QueryClientProvider>,
			);

			expect(watchBuildLogs).toHaveBeenCalledWith(
				MockWorkspace.latest_build.id,
				expect.anything(),
			);
			if (agentLogs) {
				expect(watchAgentLogs).toHaveBeenCalledWith(
					MockWorkspaceAgent.id,
					expect.anything(),
				);
			} else {
				expect(watchAgentLogs).not.toHaveBeenCalled();
			}
		},
	);
});
