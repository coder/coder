import { screen } from "@testing-library/react"
import WS from "jest-websocket-mock"
import {
  MockWorkspace,
  MockWorkspaceBuild,
  renderWithAuth,
} from "../../testHelpers/renderHelpers"
import { WorkspaceBuildPage } from "./WorkspaceBuildPage"

describe("WorkspaceBuildPage", () => {
  test("the mock server seamlessly handles JSON protocols", async () => {
    const server = new WS("ws://localhost:1234", { jsonProtocol: true })
    const client = new WebSocket("ws://localhost:1234")

    await server.connected
    const log = {
      id: "70459334-4878-4bda-a546-98eee166c4c6",
      created_at: "2022-05-19T16:46:02.283Z",
      log_source: "provisioner_daemon",
      log_level: "info",
      stage: "Another stage",
      output: "",
    }
    client.send(JSON.stringify(log))
    await expect(server).toReceiveMessage(log)
    expect(server).toHaveReceivedMessages([log])

    client.onmessage = async () => {
      renderWithAuth(<WorkspaceBuildPage />, {
        route: `/@${MockWorkspace.owner_name}/${MockWorkspace.name}/builds/${MockWorkspace.latest_build.build_number}`,
        path: "/@:username/:workspace/builds/:buildNumber",
      })

      await screen.findByText(MockWorkspaceBuild.workspace_name)
      await screen.findByText(log.stage)
    }

    server.close()
  })
})
