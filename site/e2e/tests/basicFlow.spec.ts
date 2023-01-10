import { test } from "@playwright/test"
import { email, password } from "../constants"
import { SignInPage } from "../pom"
import { clickButton, buttons, fillInput } from "../helpers"

test("Basic flow", async ({ baseURL, page }) => {
  test.slow()
  await page.goto(baseURL + "/", { waitUntil: "networkidle" })

  // Log-in with the default credentials we set up in the development server
  const signInPage = new SignInPage(baseURL, page)
  await signInPage.submitBuiltInAuthentication(email, password)

  // create Docker template
  await page.waitForSelector("text=Templates")
  await page.click("text=Templates")

  await clickButton(page, buttons.starterTemplates)

  await page.click(`text=${buttons.dockerTemplate}`)

  await clickButton(page, buttons.useTemplate)

  await clickButton(page, buttons.createTemplate)

  // create workspace
  await clickButton(page, buttons.createWorkspace)

  await fillInput(page, "Workspace Name", "my-workspace")
  await clickButton(page, buttons.submitCreateWorkspace)

  // stop workspace
  await page.waitForSelector("text=Started")
  await clickButton(page, buttons.stopWorkspace)

  // start workspace
  await page.waitForSelector("text=Stopped")
  await clickButton(page, buttons.startWorkspace)
  await page.waitForSelector("text=Started")
})
