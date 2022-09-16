import { test } from "@playwright/test"
import { email, password } from "../constants"
import { SignInPage } from "../pom"

test("Login takes user to /workspaces", async ({ baseURL, page }) => {
  await page.goto(baseURL + "/", { waitUntil: "networkidle" })

  // Log-in with the default credentials we set up in the development server
  const signInPage = new SignInPage(baseURL, page)
  await signInPage.submitBuiltInAuthentication(email, password)

  await page.waitForSelector("text=Workspaces")
})
