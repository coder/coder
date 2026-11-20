import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import * as apiModule from "#/api/api";
import { API } from "#/api/api";
import {
	MockWorkspace,
	MockWorkspaceAgent,
	MockWorkspaceAgentLogs,
	MockWorkspaceBuild,
} from "#/testHelpers/entities";
import { renderWithAuth } from "#/testHelpers/renderHelpers";
import {
	createMockWebSocket,
	type MockWebSocketServer,
} from "#/testHelpers/websockets";
import { OneWayWebSocket } from "#/utils/OneWayWebSocket";
import WorkspaceBuildPage from "./WorkspaceBuildPage";
import { LOGS_TAB_KEY } from "./WorkspaceBuildPageView";

describe("WorkspaceBuildPage", () => {
	test("gets the right workspace build", async () => {
		const getWorkspaceBuildSpy = vi
			.spyOn(API, "getWorkspaceBuildByNumber")
			.mockResolvedValue(MockWorkspaceBuild);
		renderWithAuth(<WorkspaceBuildPage />, {
			route: `/@${MockWorkspace.owner_name}/${MockWorkspace.name}/builds/${MockWorkspace.latest_build.build_number}`,
			path: "/:username/:workspace/builds/:buildNumber",
		});
		await waitFor(() =>
			expect(getWorkspaceBuildSpy).toBeCalledWith(
				MockWorkspace.owner_name,
				MockWorkspace.name,
				MockWorkspaceBuild.build_number,
			),
		);
	});

	test("shows selected agent logs", async () => {
		let mockServer: MockWebSocketServer | undefined;

		vi.spyOn(apiModule, "watchWorkspaceAgentLogs").mockImplementation(
			(agentId, params) => {
				return new OneWayWebSocket({
					apiRoute: `/api/v2/workspaceagents/${agentId}/logs`,
					searchParams: new URLSearchParams({
						follow: "true",
						after: params?.after?.toString() || "0",
					}),
					websocketInit: (url, protocol) => {
						const [socket, server] = createMockWebSocket(url, protocol);
						mockServer = server;
						return socket;
					},
				});
			},
		);
		const user = userEvent.setup();

		renderWithAuth(<WorkspaceBuildPage />, {
			route: `/@${MockWorkspace.owner_name}/${MockWorkspace.name}/builds/${MockWorkspace.latest_build.build_number}?${LOGS_TAB_KEY}=${MockWorkspaceAgent.id}`,
			path: "/:username/:workspace/builds/:buildNumber",
		});

		await screen.findByText(`Build #${MockWorkspaceBuild.build_number}`);

		await user.click(
			screen.getByRole("tab", {
				name: `coder_agent.${MockWorkspaceAgent.name}`,
			}),
		);

		expect(mockServer).toBeDefined();
		mockServer?.publishMessage(
			new MessageEvent("message", {
				data: JSON.stringify(MockWorkspaceAgentLogs),
			}),
		);

		await screen.findByText(MockWorkspaceAgentLogs[0].output);
	});
});
