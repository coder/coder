import { render, waitFor } from "@testing-library/react";
import { QueryClientProvider } from "react-query";
import { afterEach, describe, expect, it, vi } from "vitest";
import * as apiModule from "#/api/api";
import { API } from "#/api/api";
import { agentLogsKey, workspaceByIdKey } from "#/api/queries/workspaces";
import type { Workspace, WorkspaceAgentLog } from "#/api/typesGenerated";
import {
	MockStartingWorkspace,
	MockWorkspace,
	MockWorkspaceAgent,
	MockWorkspaceAgentLogs,
	MockWorkspaceAgentStarting,
} from "#/testHelpers/entities";
import { createTestQueryClient } from "#/testHelpers/renderHelpers";
import { createMockWebSocket } from "#/testHelpers/websockets";
import { OneWayWebSocket } from "#/utils/OneWayWebSocket";
import { ChatWorkspaceContext } from "../../../context/ChatWorkspaceContext";
import { WorkspaceAgentLogSection } from "./WorkspaceAgentLogSection";

afterEach(() => {
	vi.restoreAllMocks();
});

type SectionProps = { status: "running" | "completed"; buildId?: string };

const renderSection = ({
	props,
	chatBuildId,
	chatAgentId = MockWorkspaceAgent.id,
	workspace,
	cachedAgentLogs,
}: {
	props: SectionProps;
	chatBuildId?: string;
	chatAgentId?: string;
	workspace: Workspace;
	cachedAgentLogs?: WorkspaceAgentLog[];
}) => {
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
	const getAgentLogs = vi
		.spyOn(API, "getWorkspaceAgentLogs")
		.mockResolvedValue(MockWorkspaceAgentLogs);
	vi.spyOn(API, "getWorkspace").mockResolvedValue(workspace);

	const queryClient = createTestQueryClient();
	queryClient.setQueryData(workspaceByIdKey(workspace.id), workspace);
	if (cachedAgentLogs) {
		queryClient.setQueryData(agentLogsKey(chatAgentId), cachedAgentLogs);
	}

	const ui = (next: SectionProps) => (
		<QueryClientProvider client={queryClient}>
			<ChatWorkspaceContext
				value={{
					workspaceId: workspace.id,
					buildId: chatBuildId,
					agentId: chatAgentId,
				}}
			>
				<WorkspaceAgentLogSection {...next} />
			</ChatWorkspaceContext>
		</QueryClientProvider>
	);
	const { rerender } = render(ui(props));

	return {
		watchAgentLogs,
		getAgentLogs,
		rerender: (next: SectionProps) => rerender(ui(next)),
	};
};

const currentBuild = MockWorkspace.latest_build.id;

describe("WorkspaceAgentLogSection", () => {
	it.each([
		{
			case: "running, bound build is current, agent in build",
			props: { status: "running" },
			chatBuildId: currentBuild,
			workspace: MockWorkspace,
			streams: true,
			fetches: false,
		},
		{
			case: "running, bound build still starting",
			props: { status: "running" },
			chatBuildId: MockStartingWorkspace.latest_build.id,
			chatAgentId: MockWorkspaceAgentStarting.id,
			workspace: MockStartingWorkspace,
			streams: false,
			fetches: false,
		},
		{
			case: "running, bound agent not in current build",
			props: { status: "running" },
			chatBuildId: currentBuild,
			chatAgentId: "agent-from-a-previous-build",
			workspace: MockWorkspace,
			streams: false,
			fetches: false,
		},
		{
			case: "running, bound build differs from current build",
			props: { status: "running" },
			chatBuildId: "build-not-yet-reflected-in-workspace",
			workspace: MockWorkspace,
			streams: false,
			fetches: false,
		},
		{
			case: "completed, result build is current",
			props: { status: "completed", buildId: currentBuild },
			workspace: MockWorkspace,
			streams: false,
			fetches: true,
		},
		{
			case: "completed, result build replaced",
			props: { status: "completed", buildId: "an-older-build" },
			workspace: MockWorkspace,
			streams: false,
			fetches: false,
		},
	] satisfies Array<{
		case: string;
		props: SectionProps;
		chatBuildId?: string;
		chatAgentId?: string;
		workspace: Workspace;
		streams: boolean;
		fetches: boolean;
	}>)("$case", async ({ streams, fetches, ...input }) => {
		const { watchAgentLogs, getAgentLogs } = renderSection(input);

		if (fetches) {
			await waitFor(() => {
				expect(getAgentLogs).toHaveBeenCalledWith(MockWorkspaceAgent.id);
			});
		} else {
			expect(getAgentLogs).not.toHaveBeenCalled();
		}
		if (streams) {
			expect(watchAgentLogs).toHaveBeenCalledWith(
				MockWorkspaceAgent.id,
				expect.anything(),
			);
		} else {
			expect(watchAgentLogs).not.toHaveBeenCalled();
		}
	});

	// Cached logs may have been fetched before the agent finished starting.
	it.each([
		{ case: "transitions from running", mountRunning: true },
		{ case: "mounts completed", mountRunning: false },
	])("refetches a cached snapshot when it $case", async ({ mountRunning }) => {
		const completed: SectionProps = {
			status: "completed",
			buildId: currentBuild,
		};
		const { getAgentLogs, rerender } = renderSection({
			props: mountRunning ? { status: "running" } : completed,
			chatBuildId: currentBuild,
			workspace: MockWorkspace,
			cachedAgentLogs: MockWorkspaceAgentLogs.slice(0, 1),
		});
		if (mountRunning) {
			expect(getAgentLogs).not.toHaveBeenCalled();
			rerender(completed);
		}

		await waitFor(() => {
			expect(getAgentLogs).toHaveBeenCalledWith(MockWorkspaceAgent.id);
		});
	});
});
