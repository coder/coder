import { expect, Locator, Page } from "@playwright/test"
import { BasePom } from "./BasePom"

export class WorkspacePage extends BasePom {
  readonly workspaceOptionsButton: Locator
  readonly deleteWorkspaceMenuItem: Locator
  readonly stopWorkspaceButton: Locator

  readonly workspaceRunningBadge: Locator
  readonly workspaceStoppedBadge: Locator
  readonly workspaceDeletedBadge: Locator

  readonly terminalButton: Locator
  readonly agentVersion: Locator
  readonly agentLifecycleReady: Locator

  readonly deleteDialogConfirmation: Locator
  readonly deleteDialogConfirm: Locator

  constructor(baseURL: string | undefined, page: Page) {
    super(baseURL, `/templates/docker/workspace`, page)

    this.workspaceOptionsButton = page.getByTestId("workspace-options-button")
    this.deleteWorkspaceMenuItem = page.getByTestId("menuitem-delete-workspace")
    this.stopWorkspaceButton = page.getByTestId("button-stop-workspace")

    this.workspaceRunningBadge = page.getByTestId(
      "badge-workspace-status-running",
    )
    this.workspaceStoppedBadge = page.getByTestId(
      "badge-workspace-status-stopped",
    )
    this.workspaceDeletedBadge = page.getByTestId(
      "badge-workspace-status-deleted",
    )
    this.terminalButton = page.getByTestId("button-terminal")
    this.agentVersion = page.getByTestId("agent-version")
    this.agentLifecycleReady = page.getByTestId("agent-lifecycle-ready")

    this.deleteDialogConfirmation = page.getByTestId(
      "delete-dialog-confirmation",
    )
    this.deleteDialogConfirm = page.getByTestId("delete-dialog-confirm")
  }

  async loaded() {
    await this.stopWorkspaceButton.waitFor({ state: "visible" })
    await expect(this.page).toHaveTitle("admin/my-first-workspace - Coder")
  }

  async stop() {
    await this.stopWorkspaceButton.click()
  }

  async delete() {
    await this.workspaceOptionsButton.click()
    await this.deleteWorkspaceMenuItem.click()

    await this.deleteDialogConfirmation.waitFor({ state: "visible" })
    await this.deleteDialogConfirmation
      .getByLabel("Name of workspace to delete")
      .fill("my-first-workspace")

    await this.page.waitForTimeout(1000) // Wait for 1s to snapshot the delete dialog before submitting
    await this.deleteDialogConfirm.click()
  }

  async isRunning() {
    await this.workspaceRunningBadge.waitFor({ state: "visible" })
    await this.terminalButton.waitFor({ state: "visible" })
    await this.agentVersion.waitFor({ state: "visible" })
    await this.agentLifecycleReady.waitFor({ state: "visible" })
  }

  async isStopped() {
    await this.workspaceStoppedBadge.waitFor({ state: "visible" })
  }

  async isDeleted() {
    await this.workspaceDeletedBadge.waitFor({ state: "visible" })
  }
}
