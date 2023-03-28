import { renderWithAuth } from "testHelpers/renderHelpers"
import TemplateVersionEditorPage from "./TemplateVersionEditorPage"
import { screen, waitFor, within } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import * as api from "api/api"
import {
  MockTemplateVersion,
  MockWorkspaceBuildLogs,
} from "testHelpers/entities"

// For some reason this component in Jest is throwing a MUI style warning so,
// since we don't need it for this test, we can mock it out
jest.mock("components/TemplateResourcesTable/TemplateResourcesTable", () => {
  return {
    TemplateResourcesTable: () => <div />,
  }
})

test("Use custom name and set it as active when publishing", async () => {
  const user = userEvent.setup()
  renderWithAuth(<TemplateVersionEditorPage />, {
    extraRoutes: [
      {
        path: "/templates/:templateId",
        element: <div />,
      },
    ],
  })
  const topbar = await screen.findByTestId("topbar")

  // Build Template
  jest.spyOn(api, "uploadTemplateFile").mockResolvedValueOnce({ hash: "hash" })
  jest
    .spyOn(api, "createTemplateVersion")
    .mockResolvedValueOnce(MockTemplateVersion)
  jest
    .spyOn(api, "getTemplateVersion")
    .mockResolvedValue({ ...MockTemplateVersion, id: "new-version-id" })
  jest.spyOn(api, "watchBuildLogs").mockImplementation((_, onMessage) => {
    onMessage(MockWorkspaceBuildLogs[0])
    return Promise.resolve()
  })
  const buildButton = within(topbar).getByRole("button", {
    name: "Build template",
  })
  await user.click(buildButton)

  // Publish
  const patchTemplateVersion = jest
    .spyOn(api, "patchTemplateVersion")
    .mockResolvedValue(MockTemplateVersion)
  const updateActiveTemplateVersion = jest
    .spyOn(api, "updateActiveTemplateVersion")
    .mockResolvedValue({ message: "" })
  await within(topbar).findByText("Success")
  const publishButton = within(topbar).getByRole("button", {
    name: "Publish version",
  })
  await user.click(publishButton)
  const publishDialog = await screen.findByTestId("dialog")
  const nameField = within(publishDialog).getByLabelText("Version name")
  await user.clear(nameField)
  await user.type(nameField, "v1.0")
  await user.click(
    within(publishDialog).getByLabelText("Promote to default version"),
  )
  await user.click(
    within(publishDialog).getByRole("button", { name: "Publish" }),
  )
  await waitFor(() => {
    expect(patchTemplateVersion).toBeCalledWith("new-version-id", {
      name: "v1.0",
    })
  })
  expect(updateActiveTemplateVersion).toBeCalledWith("test-template", {
    id: "new-version-id",
  })
})

test("Do not mark as active if promote is not checked", async () => {
  const user = userEvent.setup()
  renderWithAuth(<TemplateVersionEditorPage />, {
    extraRoutes: [
      {
        path: "/templates/:templateId",
        element: <div />,
      },
    ],
  })
  const topbar = await screen.findByTestId("topbar")

  // Build Template
  jest.spyOn(api, "uploadTemplateFile").mockResolvedValueOnce({ hash: "hash" })
  jest
    .spyOn(api, "createTemplateVersion")
    .mockResolvedValueOnce(MockTemplateVersion)
  jest
    .spyOn(api, "getTemplateVersion")
    .mockResolvedValue({ ...MockTemplateVersion, id: "new-version-id" })
  jest.spyOn(api, "watchBuildLogs").mockImplementation((_, onMessage) => {
    onMessage(MockWorkspaceBuildLogs[0])
    return Promise.resolve()
  })
  const buildButton = within(topbar).getByRole("button", {
    name: "Build template",
  })
  await user.click(buildButton)

  // Publish
  const patchTemplateVersion = jest
    .spyOn(api, "patchTemplateVersion")
    .mockResolvedValue(MockTemplateVersion)
  const updateActiveTemplateVersion = jest
    .spyOn(api, "updateActiveTemplateVersion")
    .mockResolvedValue({ message: "" })
  await within(topbar).findByText("Success")
  const publishButton = within(topbar).getByRole("button", {
    name: "Publish version",
  })
  await user.click(publishButton)
  const publishDialog = await screen.findByTestId("dialog")
  const nameField = within(publishDialog).getByLabelText("Version name")
  await user.clear(nameField)
  await user.type(nameField, "v1.0")
  await user.click(
    within(publishDialog).getByRole("button", { name: "Publish" }),
  )
  await waitFor(() => {
    expect(patchTemplateVersion).toBeCalledWith("new-version-id", {
      name: "v1.0",
    })
  })
  expect(updateActiveTemplateVersion).toBeCalledTimes(0)
})

test("The default version name is used when a new one is not used", async () => {
  const user = userEvent.setup()
  renderWithAuth(<TemplateVersionEditorPage />, {
    extraRoutes: [
      {
        path: "/templates/:templateId",
        element: <div />,
      },
    ],
  })
  const topbar = await screen.findByTestId("topbar")

  // Build Template
  jest.spyOn(api, "uploadTemplateFile").mockResolvedValueOnce({ hash: "hash" })
  jest
    .spyOn(api, "createTemplateVersion")
    .mockResolvedValueOnce(MockTemplateVersion)
  jest
    .spyOn(api, "getTemplateVersion")
    .mockResolvedValue({ ...MockTemplateVersion, id: "new-version-id" })
  jest.spyOn(api, "watchBuildLogs").mockImplementation((_, onMessage) => {
    onMessage(MockWorkspaceBuildLogs[0])
    return Promise.resolve()
  })
  const buildButton = within(topbar).getByRole("button", {
    name: "Build template",
  })
  await user.click(buildButton)

  // Publish
  const patchTemplateVersion = jest
    .spyOn(api, "patchTemplateVersion")
    .mockResolvedValue(MockTemplateVersion)
  await within(topbar).findByText("Success")
  const publishButton = within(topbar).getByRole("button", {
    name: "Publish version",
  })
  await user.click(publishButton)
  const publishDialog = await screen.findByTestId("dialog")
  await user.click(
    within(publishDialog).getByRole("button", { name: "Publish" }),
  )
  await waitFor(() => {
    expect(patchTemplateVersion).toBeCalledWith("new-version-id", {
      name: MockTemplateVersion.name,
    })
  })
})
