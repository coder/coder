import { screen } from "@testing-library/react"
import * as API from "../../api/api"
import {
  MockWorkspace,
  MockWorkspaceBuild,
  MockWorkspaceBuildLogs,
  renderWithAuth,
} from "../../testHelpers/renderHelpers"
import { WorkspaceBuildPage } from "./WorkspaceBuildPage"

describe("WorkspaceBuildPage", () => {
  it("renders the stats and logs", async () => {
    jest.spyOn(API, "streamWorkspaceBuildLogs").mockResolvedValueOnce({
      read() {
        return Promise.resolve({
          value: undefined,
          done: true,
        })
      },
      releaseLock: jest.fn(),
      closed: Promise.resolve(undefined),
      cancel: jest.fn(),
    })
    renderWithAuth(<WorkspaceBuildPage />, {
      route: `/@${MockWorkspace.owner_name}/${MockWorkspace.name}/builds/${MockWorkspace.latest_build.build_number}`,
      path: "/@:username/:workspace/builds/:buildNumber",
    })

    await screen.findByText(MockWorkspaceBuild.workspace_name)
    await screen.findByText(MockWorkspaceBuildLogs[0].stage)
  })
})
