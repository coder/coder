import { screen } from "@testing-library/react"
import WS from "jest-websocket-mock"
import { MockWorkspace, MockWorkspaceBuild, renderWithAuth } from "../../testHelpers/renderHelpers"
import { WorkspaceBuildPage } from "./WorkspaceBuildPage"

describe("WorkspaceBuildPage", () => {
  it("renders the stats and logs", async () => {
    const server = new WS(`ws://localhost/api/v2/workspacebuilds/${MockWorkspaceBuild.id}/logs`)
    renderWithAuth(<WorkspaceBuildPage />, {
      route: `/@${MockWorkspace.owner_name}/${MockWorkspace.name}/builds/${MockWorkspace.latest_build.build_number}`,
    })
    await server.connected
    const log = {
      id: "70459334-4878-4bda-a546-98eee166c4c6",
      created_at: "2022-05-19T16:46:02.283Z",
      log_source: "provisioner_daemon",
      log_level: "info",
      stage: "Another stage",
      output: "",
    }
    server.send(JSON.stringify(log))
    await screen.findByText(MockWorkspaceBuild.workspace_name)
    await screen.findByText(log.stage)
    server.close()
  })
})
