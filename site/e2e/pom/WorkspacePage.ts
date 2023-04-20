import { expect, Locator, Page } from "@playwright/test"
import { BasePom } from "./BasePom"

export class WorkspacePage extends BasePom {
  readonly workspaceRunningBadge: Locator
  readonly workspaceStoppedBadge: Locator
  readonly terminalButton: Locator
  readonly agentVersion: Locator
  readonly agentLifecycleReady: Locator
  readonly stopWorkspaceButton: Locator

  constructor(baseURL: string | undefined, page: Page) {
    super(baseURL, `/templates/docker/workspace`, page)

    this.workspaceRunningBadge = page.getByTestId(
      "badge-workspace-status-running",
    )
    this.workspaceStoppedBadge = page.getByTestId(
      "badge-workspace-status-stopped",
    )
    this.terminalButton = page.getByTestId("button-terminal")
    this.agentVersion = page.getByTestId("agent-version")
    this.agentLifecycleReady = page.getByTestId("agent-lifecycle-ready")
    this.stopWorkspaceButton = page.getByTestId("button-stop-workspace")
  }

  async loaded() {
    await this.stopWorkspaceButton.waitFor({ state: "visible" })
    await expect(this.page).toHaveTitle("admin/my-first-workspace - Coder")
  }

  async isRunning() {
    await this.workspaceRunningBadge.waitFor({ state: "visible" })
    await this.terminalButton.waitFor({ state: "visible" })
    await this.agentVersion.waitFor({ state: "visible" })
    await this.agentLifecycleReady.waitFor({ state: "visible" })
    await this.page.waitForTimeout(1000) // Wait for 1s to snapshot the agent status on the video
  }

  async stop() {
    await this.stopWorkspaceButton.click()
  }

  async isStopped() {
    await this.workspaceStoppedBadge.waitFor({ state: "visible" })
  }
}
