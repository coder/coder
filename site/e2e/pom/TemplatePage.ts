import { expect, Locator, Page } from "@playwright/test"
import { BasePom } from "./BasePom"

export class TemplatePage extends BasePom {
  readonly createWorkspaceButton: Locator

  constructor(baseURL: string | undefined, page: Page) {
    super(baseURL, `/templates/docker`, page)

    this.createWorkspaceButton = page.getByTestId("button-create-workspace")
  }

  async loaded() {
    await this.createWorkspaceButton.waitFor({ state: "visible" })

    await expect(this.page).toHaveTitle("My First Template · Template - Coder")
  }

  async createWorkspace() {
    await this.createWorkspaceButton.click()
  }
}
