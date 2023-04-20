import { expect, Locator, Page } from "@playwright/test"
import { BasePom } from "./BasePom"

export class WorkspacePage extends BasePom {
  readonly workspaceRunningBadge: Locator
  readonly workspaceStoppedBadge: Locator
  readonly stopWorkspaceButton: Locator

  constructor(baseURL: string | undefined, page: Page) {
    super(baseURL, `/templates/docker/workspace`, page)

    this.workspaceRunningBadge = page.getByTestId("badge-workspace-status-running")
    this.workspaceStoppedBadge = page.getByTestId("badge-workspace-status-stopped")
    this.stopWorkspaceButton = page.getByTestId("button-stop-workspace")
  }

  async loaded() {
    await this.stopWorkspaceButton.waitFor({ state: "visible" })
    await expect(this.page).toHaveTitle("admin/my-first-workspace - Coder")
  }

  async isRunning() {
    await this.workspaceRunningBadge.waitFor({ state: "visible" })
  }

  async stop() {
    await this.stopWorkspaceButton.click()
  }

  async isStopped() {
    await this.workspaceStoppedBadge.waitFor({ state: "visible" })
  }
}
