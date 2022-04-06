import { test } from "@playwright/test"
import { email, password } from "../constants"
import { SignInPage, TemplatesPage } from "../pom"
import { waitForClientSideNavigation } from "./../util"

test("Login takes user to /templates", async ({ baseURL, page }) => {
  await page.goto(baseURL + "/", { waitUntil: "networkidle" })

  // Log-in with the default credentials we set up in the development server
  const signInPage = new SignInPage(baseURL, page)
  await signInPage.submitBuiltInAuthentication(email, password)

  const templatesPage = new TemplatesPage(baseURL, page)
  await waitForClientSideNavigation(page, { to: templatesPage.url })

  await page.waitForSelector("text=Templates")
})
