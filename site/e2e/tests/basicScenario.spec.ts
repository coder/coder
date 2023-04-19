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

  await test.step("Start a workspace", async() => {
    await templatePage.createWorkspace()
    await createWorkspacePage.loaded()

    await createWorkspacePage.submitForm()
    await workspacePage.loaded()
  })

  // await test.step("Workspace is up and running", async() => {
  //   await workspacePage.isRunning()
  // })

  await test.step("Finally", async() => {
    await page.waitForTimeout(5 * 60 * 1000) // FIXME
  })
})
