import { act, render, screen, waitFor } from "@testing-library/react";
import { QueryClientProvider } from "react-query";
import { afterEach, describe, expect, it, vi } from "vitest";
import * as apiModule from "#/api/api";
import { API } from "#/api/api";
import { workspaceBuildByIdKey } from "#/api/queries/workspaceBuilds";
import { workspaceByIdKey } from "#/api/queries/workspaces";
import type { ProvisionerJobLog, Workspace } from "#/api/typesGenerated";
import {
	MockStoppedWorkspace,
	MockStoppingWorkspace,
	MockWorkspace,
	MockWorkspaceAgent,
	MockWorkspaceBuildLogs,
} from "#/testHelpers/entities";
import { createTestQueryClient } from "#/testHelpers/renderHelpers";
import { createMockWebSocket } from "#/testHelpers/websockets";
import { OneWayWebSocket } from "#/utils/OneWayWebSocket";
import { ChatWorkspaceContext } from "../../../context/ChatWorkspaceContext";
import { useWorkspaceWatch } from "../../ChatConversation/useWorkspaceWatch";
import { WorkspaceBuildLogSection } from "./WorkspaceBuildLogSection";

afterEach(() => {
	vi.restoreAllMocks();
});

describe("WorkspaceBuildLogSection", () => {
	it.each([
		{ workspace: MockStoppingWorkspace, streams: true },
		{ workspace: MockStoppedWorkspace, streams: false },
	])(
		"polled build with status $workspace.latest_build.status streams: $streams",
		({ workspace, streams }) => {
			const watchBuildLogs = vi
				.spyOn(apiModule, "watchBuildLogsByBuildId")
				.mockImplementation(() => createMockWebSocket("ws://test")[0]);
			vi.spyOn(API, "getWorkspace").mockResolvedValue(workspace);
			const queryClient = createTestQueryClient();
			queryClient.setQueryData(workspaceByIdKey(workspace.id), workspace);

			render(
				<QueryClientProvider client={queryClient}>
					<ChatWorkspaceContext value={{ workspaceId: workspace.id }}>
						<WorkspaceBuildLogSection status="running" />
					</ChatWorkspaceContext>
				</QueryClientProvider>,
			);

			if (streams) {
				expect(watchBuildLogs).toHaveBeenCalledWith(
					workspace.latest_build.id,
					expect.anything(),
				);
			} else {
				expect(watchBuildLogs).not.toHaveBeenCalled();
			}
		},
	);

	it("fetches completed build metadata when the workspace lookup fails", async () => {
		const mockOverflowedBuild = {
			...MockWorkspace.latest_build,
			job: { ...MockWorkspace.latest_build.job, logs_overflowed: true },
		};
		vi.spyOn(API, "getWorkspace").mockRejectedValue(new Error("Unavailable"));
		vi.spyOn(API, "getWorkspaceBuild").mockResolvedValue(mockOverflowedBuild);
		vi.spyOn(API, "getWorkspaceBuildLogs").mockResolvedValue(
			MockWorkspaceBuildLogs,
		);
		const queryClient = createTestQueryClient();

		render(
			<QueryClientProvider client={queryClient}>
				<ChatWorkspaceContext value={{ workspaceId: MockWorkspace.id }}>
					<WorkspaceBuildLogSection
						status="completed"
						buildId={mockOverflowedBuild.id}
					/>
				</ChatWorkspaceContext>
			</QueryClientProvider>,
		);

		await waitFor(() => {
			expect(
				queryClient.getQueryData(workspaceBuildByIdKey(mockOverflowedBuild.id)),
			).toEqual(mockOverflowedBuild);
		});
	});

	it("retains overflowed build metadata when the workspace watch advances to a newer build", async () => {
		const mockOverflowedWorkspace: Workspace = {
			...MockWorkspace,
			latest_build: {
				...MockWorkspace.latest_build,
				job: { ...MockWorkspace.latest_build.job, logs_overflowed: true },
			},
		};
		const mockWorkspaceWithNewerBuild: Workspace = {
			...MockWorkspace,
			latest_build: {
				...MockWorkspace.latest_build,
				id: "new-build-id",
				build_number: MockWorkspace.latest_build.build_number + 1,
			},
		};
		const [socket, server] = createMockWebSocket("ws://test");
		vi.spyOn(apiModule, "watchWorkspace").mockImplementation(
			(workspaceId) =>
				new OneWayWebSocket({
					apiRoute: `/api/v2/workspaces/${workspaceId}/watch-ws`,
					websocketInit: () => socket,
				}),
		);
		vi.spyOn(API, "getWorkspace").mockImplementation(
			() => new Promise(() => {}),
		);
		const getWorkspaceBuild = vi
			.spyOn(API, "getWorkspaceBuild")
			.mockImplementation(() => new Promise(() => {}));
		vi.spyOn(API, "getWorkspaceBuildLogs").mockResolvedValue(
			MockWorkspaceBuildLogs,
		);
		const queryClient = createTestQueryClient();
		queryClient.setQueryData(
			workspaceByIdKey(MockWorkspace.id),
			mockOverflowedWorkspace,
		);
		const WatchedLogSection: React.FC = () => {
			useWorkspaceWatch({
				workspaceId: MockWorkspace.id,
				agentId: "test-chat",
				chatAgentId: MockWorkspaceAgent.id,
			});
			return (
				<WorkspaceBuildLogSection
					status="completed"
					buildId={mockOverflowedWorkspace.latest_build.id}
				/>
			);
		};

		render(
			<QueryClientProvider client={queryClient}>
				<ChatWorkspaceContext value={{ workspaceId: MockWorkspace.id }}>
					<WatchedLogSection />
				</ChatWorkspaceContext>
			</QueryClientProvider>,
		);

		act(() => {
			server.publishMessage(
				new MessageEvent("message", {
					data: JSON.stringify({
						type: "data",
						data: mockWorkspaceWithNewerBuild,
					}),
				}),
			);
		});

		await waitFor(() => {
			expect(getWorkspaceBuild).toHaveBeenCalledWith(
				mockOverflowedWorkspace.latest_build.id,
			);
		});
		expect(
			queryClient.getQueryData(
				workspaceBuildByIdKey(mockOverflowedWorkspace.latest_build.id),
			),
		).toEqual(mockOverflowedWorkspace.latest_build);
	});

	it("does not scroll the transcript as the build log streams and completes", async () => {
		const scrollIntoView = vi.spyOn(HTMLElement.prototype, "scrollIntoView");
		let publishLog: ((log: ProvisionerJobLog) => void) | undefined;
		vi.spyOn(apiModule, "watchBuildLogsByBuildId").mockImplementation(
			(_buildId, { onMessage }) => {
				publishLog = onMessage;
				return createMockWebSocket("ws://test")[0];
			},
		);
		vi.spyOn(API, "getWorkspaceBuildLogs").mockResolvedValue(
			MockWorkspaceBuildLogs,
		);
		const buildId = MockWorkspace.latest_build.id;
		const queryClient = createTestQueryClient();
		const ui = (
			props: React.ComponentProps<typeof WorkspaceBuildLogSection>,
		) => (
			<QueryClientProvider client={queryClient}>
				<ChatWorkspaceContext
					value={{ workspaceId: MockWorkspace.id, buildId }}
				>
					<WorkspaceBuildLogSection {...props} />
				</ChatWorkspaceContext>
			</QueryClientProvider>
		);

		const { rerender } = render(ui({ status: "running" }));
		act(() => publishLog?.(MockWorkspaceBuildLogs[0]));
		act(() => publishLog?.(MockWorkspaceBuildLogs[1]));

		// The completed call's fetched logs replace the streamed ones.
		rerender(ui({ status: "completed", buildId }));
		await screen.findByText(/Apply complete!/);

		expect(scrollIntoView).not.toHaveBeenCalled();
	});
});
