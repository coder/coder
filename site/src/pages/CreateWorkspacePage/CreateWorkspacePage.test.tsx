/* eslint-disable @typescript-eslint/no-floating-promises */
import { screen } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import * as API from "api/api"
import i18next from "i18next"
import { Language as FooterLanguage } from "../../components/FormFooter/FormFooter"
import { MockTemplate, MockWorkspace } from "../../testHelpers/entities"
import { renderWithAuth } from "../../testHelpers/renderHelpers"
import CreateWorkspacePage from "./CreateWorkspacePage"

const { t } = i18next
const nameLabelText = t("createWorkspacePage.nameLabel")
const ownerLabelText = t("createWorkspacePage.ownerLabel")

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

  it("succeeds using default owner", async () => {
    jest.spyOn(API, "createWorkspace").mockResolvedValueOnce(MockWorkspace)

    renderCreateWorkspacePage()

    const nameField = await screen.findByLabelText(nameLabelText)
    userEvent.type(nameField, "test")
    const submitButton = screen.getByText(FooterLanguage.defaultSubmitLabel)
    userEvent.click(submitButton)
  })

  it("succeeds with a specified owner", async () => {
    jest.spyOn(API, "createWorkspace").mockResolvedValueOnce(MockWorkspace)

    renderCreateWorkspacePage()

    const nameField = await screen.findByLabelText(nameLabelText)
    userEvent.type(nameField, "test")
    const ownerField = await screen.findByLabelText(ownerLabelText)
    userEvent.type(ownerField, "test")
  })
})
