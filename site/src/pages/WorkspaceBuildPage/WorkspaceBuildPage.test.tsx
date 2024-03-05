import { screen, waitFor } from "@testing-library/react";
import WS from "jest-websocket-mock";
import * as API from "api/api";
import {
  MockWorkspace,
  MockWorkspaceAgent,
  MockWorkspaceAgentLogs,
  MockWorkspaceBuild,
} from "testHelpers/entities";
import { renderWithAuth } from "testHelpers/renderHelpers";
import { WorkspaceBuildPage } from "./WorkspaceBuildPage";
import { LOGS_TAB_KEY } from "./WorkspaceBuildPageView";

afterEach(() => {
  WS.clean();
});

describe("WorkspaceBuildPage", () => {
  test("gets the right workspace build", async () => {
    const getWorkspaceBuildSpy = jest
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

  test("the mock server seamlessly handles JSON protocols", async () => {
    const server = new WS("ws://localhost:1234", { jsonProtocol: true });
    const client = new WebSocket("ws://localhost:1234");

    await server.connected;
    const log = {
      id: "70459334-4878-4bda-a546-98eee166c4c6",
      created_at: "2022-05-19T16:46:02.283Z",
      log_source: "provisioner_daemon",
      log_level: "info",
      stage: "Another stage",
      output: "",
    };
    client.send(JSON.stringify(log));
    await expect(server).toReceiveMessage(log);
    expect(server).toHaveReceivedMessages([log]);

    client.onmessage = async () => {
      renderWithAuth(<WorkspaceBuildPage />, {
        route: `/@${MockWorkspace.owner_name}/${MockWorkspace.name}/builds/${MockWorkspace.latest_build.build_number}`,
        path: "/:username/:workspace/builds/:buildNumber",
      });

      await screen.findByText(MockWorkspaceBuild.workspace_name);
      await screen.findByText(log.stage);
    };

    server.close();
  });

  test("shows selected agent logs", async () => {
    const server = new WS(
      `ws://localhost/api/v2/workspaceagents/${MockWorkspaceAgent.id}/logs?follow&after=0`,
    );
    renderWithAuth(<WorkspaceBuildPage />, {
      route: `/@${MockWorkspace.owner_name}/${MockWorkspace.name}/builds/${MockWorkspace.latest_build.build_number}?${LOGS_TAB_KEY}=${MockWorkspaceAgent.id}`,
      path: "/:username/:workspace/builds/:buildNumber",
    });
    await screen.findByText(`Build #${MockWorkspaceBuild.build_number}`);
    await server.connected;
    server.send(JSON.stringify(MockWorkspaceAgentLogs));
    await screen.findByText(MockWorkspaceAgentLogs[0].output);
  });
});
