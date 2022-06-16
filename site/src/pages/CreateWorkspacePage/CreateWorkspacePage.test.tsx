import { screen, waitFor } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import * as API from "../../api/api"
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

const fillForm = async ({ name = "example" }: { name?: string }) => {
  const nameField = await screen.findByLabelText(Language.nameLabel)
  await userEvent.type(nameField, name)
  const submitButton = await screen.findByText(FooterLanguage.defaultSubmitLabel)
  await userEvent.click(submitButton)
}

describe("CreateWorkspacePage", () => {
  it("renders", async () => {
    renderCreateWorkspacePage()
    const element = await screen.findByText("Create workspace")
    expect(element).toBeDefined()
  })

  it("succeeds", async () => {
    renderCreateWorkspacePage()
    // You have to spy the method before it is used.
    jest.spyOn(API, "createWorkspace").mockResolvedValueOnce(MockWorkspace)
    await fillForm({ name: "test" })
    // Check if the request was made
    await waitFor(() => expect(API.createWorkspace).toBeCalledTimes(1))
  })
})
