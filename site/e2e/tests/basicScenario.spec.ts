import { test } from "@playwright/test"
import { getStatePath } from "../helpers"
import { TemplatesPage } from "../pom/TemplatesPage"
import { CreateTemplatePage } from "../pom/CreateTemplatePage"
import { TemplatePage } from "../pom/TemplatePage"
import { CreateWorkspacePage } from "../pom/CreateWorkspacePage"
import { WorkspacePage } from "../pom/WorkspacePage"

test.use({ storageState: getStatePath("authState") })

test("Basic scenario", async ({ page, baseURL }) => {
  const templatesPage = new TemplatesPage(baseURL, page)
  const createTemplatePage = new CreateTemplatePage(baseURL, page)
  const templatePage = new TemplatePage(baseURL, page)
  const createWorkspacePage = new CreateWorkspacePage(baseURL, page)
  const workspacePage = new WorkspacePage(baseURL, page)

  await test.step("Load empty templates page", async () => {
    await templatesPage.goto()
    await templatesPage.loaded()
  })

  await test.step("Upload a template", async () => {
    await templatesPage.addTemplate()
    await createTemplatePage.loaded()

    await createTemplatePage.submitForm()
    await templatePage.loaded()
  })

  await test.step("Start a workspace", async () => {
    await templatePage.createWorkspace()
    await createWorkspacePage.loaded()

    await createWorkspacePage.submitForm()
    await workspacePage.loaded()
    await workspacePage.isRunning()
    await page.waitForTimeout(1000) // Wait for 1s to snapshot the agent status on the video
  })

  await test.step("Stop the workspace", async () => {
    await workspacePage.stop()
    await workspacePage.isStopped()
  })

  await test.step("Delete the workspace", async () => {
    await workspacePage.delete()
    await workspacePage.isDeleted()
    await page.waitForTimeout(1000) // Wait to show the deleted workspace
  })
})
