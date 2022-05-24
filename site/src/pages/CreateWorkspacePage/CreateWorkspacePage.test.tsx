import { screen, waitFor } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import React from "react"
import { reach, StringSchema } from "yup"
import * as API from "../../api/api"
import { Language as FooterLanguage } from "../../components/FormFooter/FormFooter"
import { MockTemplate, MockWorkspace } from "../../testHelpers/entities"
import { renderWithAuth } from "../../testHelpers/renderHelpers"
import CreateWorkspacePage from "./CreateWorkspacePage"
import { Language, validationSchema } from "./CreateWorkspacePageView"

const renderCreateWorkspacePage = () => {
  return renderWithAuth(<CreateWorkspacePage />, {
    route: "/workspaces/new?template=" + MockTemplate.name,
    path: "/workspaces/new",
  })
}

const fillForm = async ({ name = "example" }: { name?: string }) => {
  const nameField = await screen.findByLabelText(Language.nameLabel)
  await userEvent.type(nameField, name)
  const submitButton = await screen.findByText(FooterLanguage.defaultSubmitLabel)
  await userEvent.click(submitButton)
}

const nameSchema = reach(validationSchema, "name") as StringSchema

describe("CreateWorkspacePage", () => {
  it("renders", async () => {
    renderCreateWorkspacePage()
    const element = await screen.findByText("Create workspace")
    expect(element).toBeDefined()
  })

  it("shows validation error message", async () => {
    renderCreateWorkspacePage()
    await fillForm({ name: "$$$" })
    const errorMessage = await screen.findByText(Language.nameMatches)
    expect(errorMessage).toBeDefined()
  })

  it("succeeds", async () => {
    renderCreateWorkspacePage()
    // You have to spy the method before it is used.
    jest.spyOn(API, "createWorkspace").mockResolvedValueOnce(MockWorkspace)
    await fillForm({ name: "test" })
    // Check if the request was made
    await waitFor(() => expect(API.createWorkspace).toBeCalledTimes(1))
  })

  describe("validationSchema", () => {
    it("allows a 1-letter name", () => {
      const validate = () => nameSchema.validateSync("t")
      expect(validate).not.toThrow()
    })

    it("allows a 32-letter name", () => {
      const input = Array(32).fill("a").join("")
      const validate = () => nameSchema.validateSync(input)
      expect(validate).not.toThrow()
    })

    it("allows 'test-3' to be used as name", () => {
      const validate = () => nameSchema.validateSync("test-3")
      expect(validate).not.toThrow()
    })

    it("allows '3-test' to be used as a name", () => {
      const validate = () => nameSchema.validateSync("3-test")
      expect(validate).not.toThrow()
    })

    it("disallows a 33-letter name", () => {
      const input = Array(33).fill("a").join("")
      const validate = () => nameSchema.validateSync(input)
      expect(validate).toThrow()
    })

    it("disallows a space", () => {
      const validate = () => nameSchema.validateSync("test 3")
      expect(validate).toThrow()
    })
  })
})
