import { act, render, screen } from "@testing-library/react";
import { QueryClientProvider } from "react-query";
import { afterEach, describe, expect, it, vi } from "vitest";
import * as apiModule from "#/api/api";
import { API } from "#/api/api";
import { workspaceByIdKey } from "#/api/queries/workspaces";
import type { ProvisionerJobLog } from "#/api/typesGenerated";
import {
	MockStoppedWorkspace,
	MockStoppingWorkspace,
	MockWorkspace,
	MockWorkspaceBuildLogs,
} from "#/testHelpers/entities";
import { createTestQueryClient } from "#/testHelpers/renderHelpers";
import { createMockWebSocket } from "#/testHelpers/websockets";
import { ChatWorkspaceContext } from "../../../context/ChatWorkspaceContext";
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

		// scrollIntoView also scrolls the chat transcript.
		expect(scrollIntoView).not.toHaveBeenCalled();
	});
});
