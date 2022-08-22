/* eslint-disable @typescript-eslint/no-floating-promises */
import { screen } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import * as API from "api/api"
import { Language as FooterLanguage } from "../../components/FormFooter/FormFooter"
import { MockTemplate, MockWorkspace } from "../../testHelpers/entities"
import { renderWithAuth } from "../../testHelpers/renderHelpers"
import CreateWorkspacePage from "./CreateWorkspacePage"
import { Language } from "./CreateWorkspacePageView"

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

  it("succeeds", async () => {
    jest.spyOn(API, "createWorkspace").mockResolvedValueOnce(MockWorkspace)

    renderCreateWorkspacePage()

    const nameField = await screen.findByLabelText(Language.nameLabel)
    userEvent.type(nameField, "test")
    const submitButton = screen.getByText(FooterLanguage.defaultSubmitLabel)
    userEvent.click(submitButton)
  })
})
