import { fireEvent, screen } from "@testing-library/react"
import {
  MockTemplateVersionParameter1,
  MockTemplateVersionParameter2,
  MockWorkspace,
  MockWorkspaceBuildParameter1,
  MockWorkspaceBuildParameter2,
  renderWithAuth,
} from "testHelpers/renderHelpers"
import * as API from "api/api"
import i18next from "i18next"
import { WorkspaceBuildParametersPage } from "./WorkspaceBuildParametersPage"

const { t } = i18next

const pageTitleText = t("title", { ns: "workspaceBuildParametersPage" })
const validationNumberNotInRangeText = t("validationNumberNotInRange", {
  ns: "workspaceBuildParametersPage",
  min: "1",
  max: "3",
})

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
    jest
      .spyOn(API, "getWorkspaceBuildParameters")
      .mockResolvedValueOnce([
        MockWorkspaceBuildParameter1,
        MockWorkspaceBuildParameter2,
      ])
    renderWorkspaceBuildParametersPage()

    const element = await screen.findByText(pageTitleText)
    expect(element).toBeDefined()

    const goBackButton = await screen.findByText("Go back")
    expect(goBackButton).toBeDefined()
  })

  it("renders with rich parameter", async () => {
    jest.spyOn(API, "getWorkspace").mockResolvedValueOnce(MockWorkspace)
    jest
      .spyOn(API, "getTemplateVersionRichParameters")
      .mockResolvedValueOnce([
        MockTemplateVersionParameter1,
        MockTemplateVersionParameter2,
      ])
    jest
      .spyOn(API, "getWorkspaceBuildParameters")
      .mockResolvedValueOnce([
        MockWorkspaceBuildParameter1,
        MockWorkspaceBuildParameter2,
      ])

    renderWorkspaceBuildParametersPage()

    const element = await screen.findByText(pageTitleText)
    expect(element).toBeDefined()

    const firstParameter = await screen.findByLabelText(
      MockTemplateVersionParameter1.name,
    )
    expect(firstParameter).toBeDefined()

    const secondParameter = await screen.findByLabelText(
      MockTemplateVersionParameter2.name,
    )
    expect(secondParameter).toBeDefined()
  })

  it("rich parameter: number validation fails", async () => {
    jest
      .spyOn(API, "getTemplateVersionRichParameters")
      .mockResolvedValueOnce([
        MockTemplateVersionParameter1,
        MockTemplateVersionParameter2,
      ])
    jest
      .spyOn(API, "getWorkspaceBuildParameters")
      .mockResolvedValueOnce([
        MockWorkspaceBuildParameter1,
        MockWorkspaceBuildParameter2,
      ])
    renderWorkspaceBuildParametersPage()

    const element = await screen.findByText(pageTitleText)
    expect(element).toBeDefined()
    const secondParameter = await screen.findByText(
      MockTemplateVersionParameter2.description,
    )
    expect(secondParameter).toBeDefined()

    const secondParameterField = await screen.findByLabelText(
      MockTemplateVersionParameter2.name,
    )
    expect(secondParameterField).toBeDefined()

    fireEvent.change(secondParameterField, {
      target: { value: "4" },
    })
    fireEvent.submit(secondParameter)

    const validationError = await screen.findByText(
      validationNumberNotInRangeText,
    )
    expect(validationError).toBeDefined()
  })
})
