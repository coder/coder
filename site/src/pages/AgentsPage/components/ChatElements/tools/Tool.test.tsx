import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClientProvider } from "react-query";
import { afterEach, describe, expect, it, vi } from "vitest";
import * as apiModule from "#/api/api";
import { API } from "#/api/api";
import { workspaceByIdKey } from "#/api/queries/workspaces";
import { MockWorkspace, MockWorkspaceAgent } from "#/testHelpers/entities";
import {
	createTestQueryClient,
	renderComponent,
} from "#/testHelpers/renderHelpers";
import { createMockWebSocket } from "#/testHelpers/websockets";
import { OneWayWebSocket } from "#/utils/OneWayWebSocket";
import { ChatWorkspaceContext } from "../../../context/ChatWorkspaceContext";
import { WorkspaceFilesContext } from "../../../context/WorkspaceFilesContext";
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
							organizationId="organization-id"
							mcpServers={[]}
							isError={false}
							subagentTitles={new Map()}
							subagentVariants={new Map()}
							shellToolDisplayMode="auto"
							codeDiffDisplayMode="auto"
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

describe("Tool generic rows", () => {
	it("falls back to the tool name when the model intent is whitespace", () => {
		renderComponent(
			<QueryClientProvider client={createTestQueryClient()}>
				<Tool
					name="custom_tool"
					organizationId="organization-id"
					mcpServers={[]}
					status="completed"
					args={{ query: "value" }}
					isError={false}
					modelIntent="   "
					subagentTitles={new Map()}
					subagentVariants={new Map()}
					shellToolDisplayMode="auto"
					codeDiffDisplayMode="auto"
				/>
			</QueryClientProvider>,
		);

		expect(screen.getByRole("button")).toHaveAccessibleName("custom_tool");
	});
});

describe("Tool file rows", () => {
	it.each([
		{ name: "read_file", args: { path: "/home/coder/main.go" }, verb: "Read" },
		{
			name: "write_file",
			args: { path: "/home/coder/main.go", content: "package main\n" },
			verb: "Wrote",
		},
	])(
		"$name opens its file without toggling the row",
		async ({ name, args, verb }) => {
			const user = userEvent.setup();
			const openFile = vi.fn();
			renderComponent(
				<QueryClientProvider client={createTestQueryClient()}>
					<WorkspaceFilesContext.Provider
						value={{
							agentId: "agent-1",
							defaultDirectory: "/home/coder",
							repoRoots: [],
							repoRootsSettled: true,
							repositories: new Map(),
							openFile,
							revealInFiles: vi.fn(),
							diffStatuses: [],
						}}
					>
						<Tool
							name={name}
							organizationId="organization-id"
							mcpServers={[]}
							status="completed"
							args={args}
							result={{ content: "package main" }}
							isError={false}
							subagentTitles={new Map()}
							subagentVariants={new Map()}
							shellToolDisplayMode="auto"
							codeDiffDisplayMode="auto"
						/>
					</WorkspaceFilesContext.Provider>
				</QueryClientProvider>,
			);

			await user.click(
				screen.getByRole("button", { name: "Open /home/coder/main.go" }),
			);

			expect(openFile).toHaveBeenCalledWith("/home/coder/main.go");
			expect(
				screen.getByRole("button", { name: `${verb} main.go` }),
			).toHaveAttribute("aria-expanded", "false");
		},
	);
});
