import { test } from "@playwright/test"
import { getStatePath } from "../helpers"
import { TemplatesPage } from "../pom/TemplatesPage"
import { CreateTemplatePage } from "../pom/CreateTemplatePage"

test.use({ storageState: getStatePath("authState") })

test("Basic scenario", async ({ page, baseURL }) => {
  const templatesPage = new TemplatesPage(baseURL, page)
  const createTemplatePage = new CreateTemplatePage(baseURL, page)

  await test.step("Load empty templates page", async () => {
    await templatesPage.goto()
    await templatesPage.loaded()
  })

  await test.step("Upload a template", async () => {
    await templatesPage.addTemplate()
    await createTemplatePage.loaded()

    await createTemplatePage.fillIn()

    await page.waitForTimeout(5 * 60 * 1000) // FIXME
  })
})
