import { test, expect } from "@playwright/test"
import { email, password } from "../constants"
import { SignInPage } from "../pom"
import { clickButton, buttons, fillInput } from "../helpers"
import dayjs from "dayjs"

test("Basic flow", async ({ baseURL, page }) => {
  // We're keeping entire flows in one test, which means the test needs extra time.
  test.setTimeout(5 * 60 * 1000)
  await page.goto(baseURL + "/", { waitUntil: "networkidle" })

  // Log-in with the default credentials we set up in the development server
  const signInPage = new SignInPage(baseURL, page)
  await signInPage.submitBuiltInAuthentication(email, password)

  // create Docker template
  await page.click("text=Templates")

  await clickButton(page, buttons.starterTemplates)

  await page.click(`text=${buttons.dockerTemplate}`)

  await clickButton(page, buttons.useTemplate)

  await clickButton(page, buttons.createTemplate)

  // create workspace
  await clickButton(page, buttons.createWorkspace)

  // give workspace a unique name to avoid failure
  await fillInput(
    page,
    "Workspace Name",
    `workspace-${dayjs().format("MM-DD-hh-mm-ss")}`,
  )
  await clickButton(page, buttons.submitCreateWorkspace)

  // stop workspace
  await clickButton(page, buttons.stopWorkspace)

  // start workspace
  await clickButton(page, buttons.startWorkspace)
  const stopButton = page.getByRole("button", {
    name: buttons.stopWorkspace,
    exact: true,
  })
  await expect(stopButton).toBeEnabled({ timeout: 60_000 })
})
