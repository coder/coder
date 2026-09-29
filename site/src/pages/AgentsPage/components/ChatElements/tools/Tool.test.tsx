import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
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
	it.each(
		[
			{ name: "start_workspace", streamsAgentLogs: true },
			{ name: "create_workspace", streamsAgentLogs: true },
			{ name: "stop_workspace", streamsAgentLogs: false },
		].flatMap((row) => [
			{ ...row, status: "running" as const },
			{ ...row, status: "completed" as const },
		]),
	)(
		"$name $status shows its build's logs, agent logs: $streamsAgentLogs",
		async ({ name, status, streamsAgentLogs }) => {
			const buildId = MockWorkspace.latest_build.id;
			const isRunning = status === "running";
			const watchBuildLogs = vi
				.spyOn(apiModule, "watchBuildLogsByBuildId")
				.mockImplementation(() => createMockWebSocket("ws://test")[0]);
			const getBuildLogs = vi
				.spyOn(API, "getWorkspaceBuildLogs")
				.mockResolvedValue([]);
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

			// Completed rows omit the binding so only the result build_id matches.
			render(
				<QueryClientProvider client={queryClient}>
					<ChatWorkspaceContext
						value={{
							workspaceId: MockWorkspace.id,
							buildId: isRunning ? buildId : undefined,
							agentId: MockWorkspaceAgent.id,
						}}
					>
						<Tool
							name={name}
							status={status}
							result={isRunning ? undefined : { build_id: buildId }}
						/>
					</ChatWorkspaceContext>
				</QueryClientProvider>,
			);
			if (!isRunning) {
				await userEvent.click(screen.getByRole("button", { expanded: false }));
			}

			if (isRunning) {
				expect(watchBuildLogs).toHaveBeenCalledWith(buildId, expect.anything());
			} else {
				await waitFor(() => {
					expect(getBuildLogs).toHaveBeenCalledWith(buildId);
				});
			}
			if (streamsAgentLogs) {
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
