import { screen } from "@testing-library/react"
import React from "react"
import { MockWorkspaceBuild, MockWorkspaceBuildLogs, renderWithAuth } from "../../testHelpers/renderHelpers"
import { WorkspaceBuildPage } from "./WorkspaceBuildPage"

describe("WorkspaceBuildPage", () => {
  it("renders the stats and logs", async () => {
    renderWithAuth(<WorkspaceBuildPage />, { route: `/builds/${MockWorkspaceBuild.id}`, path: "/builds/:buildId" })

    await screen.findByText(MockWorkspaceBuild.workspace_id)
    await screen.findByText(MockWorkspaceBuildLogs[0].stage)
  })
})
