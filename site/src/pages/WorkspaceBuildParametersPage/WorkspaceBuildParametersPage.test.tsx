import { screen, waitFor } from "@testing-library/react"
import {
  MockTemplateVersionParameter1,
  MockWorkspace,
  MockWorkspaceBuildParameter1,
  MockWorkspaceBuildParameter2,
  renderWithAuth,
} from "testHelpers/renderHelpers"
import * as API from "api/api"
import { WorkspaceBuildParametersPage } from "./WorkspaceBuildParametersPage"

const renderWorkspaceBuildParametersPage = () => {
  return renderWithAuth(<WorkspaceBuildParametersPage />, {
    route: `/@${MockWorkspace.owner_name}/${MockWorkspace.name}/build-parameters`,
    path: `/@:ownerName/:workspaceName/build-parameters`,
  })
}

describe("WorkspaceBuildParametersPage", () => {
  it("renders without rich parameters", async () => {
    jest.spyOn(API, "getWorkspace").mockResolvedValueOnce(MockWorkspace)
    jest
      .spyOn(API, "getTemplateVersionRichParameters")
      .mockResolvedValueOnce([])

    await waitFor(() => renderWorkspaceBuildParametersPage())

    const element = screen.findByDisplayValue("Workspace build parameters")
    expect(element).toBeDefined()

    const goBackButton = screen.findByDisplayValue("Go back")
    expect(goBackButton).toBeDefined()
  })

  it("renders with rich parameter", async () => {
    jest.spyOn(API, "getWorkspace").mockResolvedValueOnce(MockWorkspace)
    jest
      .spyOn(API, "getTemplateVersionRichParameters")
      .mockResolvedValueOnce([MockTemplateVersionParameter1])
    jest
      .spyOn(API, "getWorkspaceBuildParameters")
      .mockResolvedValueOnce([
        MockWorkspaceBuildParameter1,
        MockWorkspaceBuildParameter2,
      ])

    await waitFor(() => renderWorkspaceBuildParametersPage())

    const element = screen.findByText("Workspace build parameters")
    expect(element).toBeDefined()

    const firstParameter = screen.findByLabelText(
      MockTemplateVersionParameter1.name,
    )
    expect(firstParameter).toBeDefined()

    const firstParameterValue = screen.findByText(
      MockWorkspaceBuildParameter1.value,
    )
    expect(firstParameterValue).toBeDefined()
  })
})
