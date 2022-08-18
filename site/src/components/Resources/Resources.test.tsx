import { screen } from "@testing-library/react"
import {
  MockStoppedWorkspace,
  MockWorkspaceResource,
  MockWorkspaceResource2,
} from "testHelpers/entities"
import { render } from "testHelpers/renderHelpers"
import { DisplayAgentStatusLanguage } from "util/workspace"
import { Resources } from "./Resources"

describe("ResourceTable", () => {
  it("hides status text when a workspace is stopped", async () => {
    // When
    const props = {
      resource: [{ ...MockWorkspaceResource }, { ...MockWorkspaceResource2 }],
      workspace: { ...MockStoppedWorkspace },
      canUpdateWorkspace: false,
    }

    render(<Resources {...props} />)

    const statusText = screen.queryByText(DisplayAgentStatusLanguage.connecting)

    // Then
    expect(statusText).toBeNull()
  })
})
