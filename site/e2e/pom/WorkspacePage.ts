import { expect, Locator, Page } from "@playwright/test"
import { BasePom } from "./BasePom"

export class WorkspacePage extends BasePom {
  readonly stopWorkspaceButton: Locator

  constructor(baseURL: string | undefined, page: Page) {
    super(baseURL, `/templates/docker/workspace`, page)

    this.stopWorkspaceButton = page.getByTestId("button-stop-workspace")
  }

  async loaded() {
    await this.stopWorkspaceButton.waitFor({ state: "visible" })

    await expect(this.page).toHaveTitle("admin/workspace-1 - Coder")
  }

  async stop() {
    await this.stopWorkspaceButton.click()
  }
}
