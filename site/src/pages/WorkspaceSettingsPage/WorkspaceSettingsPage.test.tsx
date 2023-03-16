import userEvent from "@testing-library/user-event"
import {
  MockTemplateVersionParameter1,
  MockTemplateVersionParameter2,
  MockWorkspace,
  MockWorkspaceBuild,
  MockWorkspaceBuildParameter1,
  MockWorkspaceBuildParameter2,
  renderWithAuth,
  waitForLoaderToBeRemoved,
} from "testHelpers/renderHelpers"
import WorkspaceSettingsPage from "./WorkspaceSettingsPage"
import { screen, waitFor, within } from "@testing-library/react"
import * as api from "api/api"

test("Submit the workspace settings page successfully", async () => {
  // Mock the API calls that loads data
  jest
    .spyOn(api, "getWorkspaceByOwnerAndName")
    .mockResolvedValueOnce(MockWorkspace)
  jest
    .spyOn(api, "getTemplateVersionRichParameters")
    .mockResolvedValueOnce([
      MockTemplateVersionParameter1,
      MockTemplateVersionParameter2,
    ])
  jest
    .spyOn(api, "getWorkspaceBuildParameters")
    .mockResolvedValueOnce([
      MockWorkspaceBuildParameter1,
      MockWorkspaceBuildParameter2,
    ])
  // Mock the API calls that submit data
  const patchWorkspaceSpy = jest
    .spyOn(api, "patchWorkspace")
    .mockResolvedValue()
  const postWorkspaceBuildSpy = jest
    .spyOn(api, "postWorkspaceBuild")
    .mockResolvedValue(MockWorkspaceBuild)
  // Setup event and rendering
  const user = userEvent.setup()
  renderWithAuth(<WorkspaceSettingsPage />, {
    route: "/@test-user/test-workspace/settings",
    path: "/@:username/:workspace/settings",
    // Need this because after submit the user is redirected
    extraRoutes: [{ path: "/@:username/:workspace", element: <div /> }],
  })
  await waitForLoaderToBeRemoved()
  // Fill the form and submit
  const form = screen.getByTestId("form")
  const name = within(form).getByLabelText("Name")
  await user.clear(name)
  await user.type(within(form).getByLabelText("Name"), "new-name")
  const parameter1 = within(form).getByLabelText(
    MockWorkspaceBuildParameter1.name,
    { exact: false },
  )
  await user.clear(parameter1)
  await user.type(parameter1, "new-value")
  const parameter2 = within(form).getByLabelText(
    MockWorkspaceBuildParameter2.name,
    { exact: false },
  )
  await user.clear(parameter2)
  await user.type(parameter2, "1")
  await user.click(within(form).getByRole("button", { name: "Submit" }))
  // Assert that the API calls were made with the correct data
  await waitFor(() => {
    expect(patchWorkspaceSpy).toHaveBeenCalledWith(MockWorkspace.id, {
      name: "new-name",
    })
    expect(postWorkspaceBuildSpy).toHaveBeenCalledWith(MockWorkspace.id, {
      transition: "start",
      rich_parameter_values: [
        { name: MockTemplateVersionParameter1.name, value: "new-value" },
        { name: MockTemplateVersionParameter2.name, value: "1" },
      ],
    })
  })
})
