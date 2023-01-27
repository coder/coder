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
  MockTemplateVersionParameter1,
  MockTemplateVersionParameter2,
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
  it("renders with rich parameter", async () => {
    jest
      .spyOn(API, "getTemplateVersionRichParameters")
      .mockResolvedValueOnce([MockTemplateVersionParameter1])

    await waitFor(() => renderCreateWorkspacePage())

    const element = screen.findByText("Create workspace")
    expect(element).toBeDefined()
    const firstParameter = screen.findByText(
      MockTemplateVersionParameter1.description,
    )
    expect(firstParameter).toBeDefined()
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
    jest
      .spyOn(API, "getTemplateVersionRichParameters")
      .mockResolvedValueOnce([MockTemplateVersionParameter1])

    await waitFor(() =>
      renderWithAuth(<CreateWorkspacePage />, {
        route:
          "/templates/" +
          MockTemplate.name +
          `/workspace?param.${param}=${paramValue}`,
        path: "/templates/:template/workspace",
      }),
    )

    await screen.findByDisplayValue(paramValue)
  })

  it("uses default rich param values passed from the URL", async () => {
    const param = "first_parameter"
    const paramValue = "It works!"
    jest.spyOn(API, "getTemplateVersionSchema").mockResolvedValueOnce([
      mockParameterSchema({
        name: param,
        default_source_value: "",
      }),
    ])
    jest
      .spyOn(API, "getTemplateVersionRichParameters")
      .mockResolvedValueOnce([MockTemplateVersionParameter1])

    await waitFor(() =>
      renderWithAuth(<CreateWorkspacePage />, {
        route:
          "/templates/" +
          MockTemplate.name +
          `/workspace?param.${param}=${paramValue}`,
        path: "/templates/:template/workspace",
      }),
    )

    await screen.findByDisplayValue(paramValue)
  })

  it("rich parameter: number validation fails", async () => {
    jest
      .spyOn(API, "getTemplateVersionRichParameters")
      .mockResolvedValueOnce([
        MockTemplateVersionParameter1,
        MockTemplateVersionParameter2,
      ])

    await waitFor(() => renderCreateWorkspacePage())

    const element = screen.findByText("Create workspace")
    expect(element).toBeDefined()
    const secondParameter = screen.findByText(
      MockTemplateVersionParameter2.description,
    )
    expect(secondParameter).toBeDefined()

    const secondParameterField = await screen.findByLabelText(
      MockTemplateVersionParameter2.name,
    )
    fireEvent.change(secondParameterField, {
      target: { value: "4" },
    })

    const validationError = screen.findByText("Value must be between")
    expect(validationError).toBeDefined()
  })
})
