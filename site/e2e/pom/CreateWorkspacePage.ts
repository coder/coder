import { expect, Locator, Page } from "@playwright/test"
import { BasePom } from "./BasePom"

export class CreateWorkspacePage extends BasePom {
  readonly createWorkspaceForm: Locator
  readonly submitButton: Locator

  constructor(baseURL: string | undefined, page: Page) {
    super(baseURL, `/templates/docker/workspace`, page)

    this.createWorkspaceForm = page.getByTestId("form-create-workspace")
    this.submitButton = page.getByTestId("button-create-workspace")
  }

  async loaded() {
    await expect(this.page).toHaveTitle("Create Workspace - Coder")

    await this.createWorkspaceForm.waitFor({ state: "visible" })
    await this.submitButton.waitFor({ state: "visible" })
  }

  async submitForm() {
    await this.createWorkspaceForm
      .getByLabel("Workspace Name")
      .fill("my-first-workspace")

    await this.submitButton.click()
  }
}
