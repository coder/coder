import { test } from "@playwright/test"
import { email, password } from "../constants"
import { SignInPage, WorkspacesPage } from "../pom"
import { waitForClientSideNavigation } from "./../util"

test("Login takes user to /workspaces", async ({ baseURL, page }) => {
  await page.goto(baseURL + "/", { waitUntil: "networkidle" })

  // Log-in with the default credentials we set up in the development server
  const signInPage = new SignInPage(baseURL, page)
  await signInPage.submitBuiltInAuthentication(email, password)

  const workspacesPage = new WorkspacesPage(baseURL, page)
  await waitForClientSideNavigation(page, { to: workspacesPage.url })

  await page.waitForSelector("text=Workspaces")
})
