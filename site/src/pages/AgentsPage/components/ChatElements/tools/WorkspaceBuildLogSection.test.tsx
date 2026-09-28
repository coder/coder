import { render } from "@testing-library/react";
import { QueryClientProvider } from "react-query";
import { afterEach, describe, expect, it, vi } from "vitest";
import * as apiModule from "#/api/api";
import { API } from "#/api/api";
import { workspaceByIdKey } from "#/api/queries/workspaces";
import {
	MockStoppedWorkspace,
	MockStoppingWorkspace,
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
});
