import { fireEvent, screen, waitFor } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import * as API from "api/api"
import i18next from "i18next"
import {
  mockParameterSchema,
  MockTemplate,
  MockUser,
  MockWorkspace,
  MockWorkspaceQuota,
  MockWorkspaceRequest,
} from "testHelpers/entities"
import { renderWithAuth } from "testHelpers/renderHelpers"
import CreateWorkspacePage from "./CreateWorkspacePage"

const { t } = i18next

const nameLabelText = t("nameLabel", { ns: "createWorkspacePage" })
const createWorkspaceText = t("createWorkspace", { ns: "createWorkspacePage" })

const renderCreateWorkspacePage = () => {
  return renderWithAuth(<CreateWorkspacePage />, {
    route: "/templates/" + MockTemplate.name + "/workspace",
    path: "/templates/:template/workspace",
  })
}

describe("CreateWorkspacePage", () => {
  it("renders", async () => {
    renderCreateWorkspacePage()
    const element = await screen.findByText("Create workspace")
    expect(element).toBeDefined()
  })

  it("succeeds with default owner", async () => {
    jest
      .spyOn(API, "getUsers")
      .mockResolvedValueOnce({ users: [MockUser], count: 1 })
    jest
      .spyOn(API, "getWorkspaceQuota")
      .mockResolvedValueOnce(MockWorkspaceQuota)
    jest.spyOn(API, "createWorkspace").mockResolvedValueOnce(MockWorkspace)

    renderCreateWorkspacePage()

    const nameField = await screen.findByLabelText(nameLabelText)

    // have to use fireEvent b/c userEvent isn't cleaning up properly between tests
    fireEvent.change(nameField, {
      target: { value: "test" },
    })

    const submitButton = screen.getByText(createWorkspaceText)
    await userEvent.click(submitButton)

    await waitFor(() =>
      expect(API.createWorkspace).toBeCalledWith(
        MockUser.organization_ids[0],
        MockUser.id,
        {
          ...MockWorkspaceRequest,
        },
      ),
    )
  })

  it("uses default param values passed from the URL", async () => {
    const param = "dotfile_uri"
    const paramValue = "localhost:3000"
    jest.spyOn(API, "getTemplateVersionSchema").mockResolvedValueOnce([
      mockParameterSchema({
        name: param,
        default_source_value: "",
      }),
    ])
    renderWithAuth(<CreateWorkspacePage />, {
      route:
        "/templates/" +
        MockTemplate.name +
        `/workspace?param.${param}=${paramValue}`,
      path: "/templates/:template/workspace",
    })
    await screen.findByDisplayValue(paramValue)
  })
})
