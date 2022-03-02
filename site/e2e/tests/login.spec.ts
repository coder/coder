import { test } from "@playwright/test"
import { SignInPage } from "../pom"
import { email, password } from "../constants"

test("Login takes user to /projects", async ({ page, baseURL }) => {
  await page.goto(baseURL + "/", { waitUntil: "networkidle" })

  // Log-in with the default credentials we set up in the development server
  const signInPage = new SignInPage(page)
  await signInPage.submitBuiltInAuthentication(email, password)

  await page.waitForNavigation({ url: baseURL + "/projects", waitUntil: "networkidle" })

  await page.waitForSelector("text=Projects")
})
