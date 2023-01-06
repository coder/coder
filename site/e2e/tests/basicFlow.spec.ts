import { test } from "@playwright/test"
import { email, password } from "../constants"
import { SignInPage } from "../pom"
import { clickButtonByText, buttons, urls, fillInput } from "../helpers";

test("Basic flow", async ({ baseURL, page }) => {
  await page.goto(baseURL + "/", { waitUntil: "networkidle" })

  // Log-in with the default credentials we set up in the development server
  const signInPage = new SignInPage(baseURL, page)
  await signInPage.submitBuiltInAuthentication(email, password)

  await page.waitForSelector("text=Workspaces")

  // create Docker template
  await page.goto(urls.templates);
  await clickButtonByText(page, buttons.starterTemplates)

  await clickButtonByText(page, buttons.dockerTemplate)

  await clickButtonByText(page, buttons.useTemplate)

  await clickButtonByText(page, buttons.createTemplate)

  // create workspace
  await page.click('span:has-text("docker")')
  await clickButtonByText(page, buttons.createWorkspace)

  await fillInput(page, "Workspace Name", "my-workspace")
  await clickButtonByText(page, buttons.submitCreateWorkspace)

  // stop workspace
  await clickButtonByText(page, buttons.stopWorkspace)

  // start workspace
  await clickButtonByText(page, buttons.startWorkspace)
})
