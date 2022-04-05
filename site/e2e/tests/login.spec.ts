import { test } from "@playwright/test"
import { email, password } from "../constants"
import { ProjectsPage, SignInPage } from "../pom"
import { waitForClientSideNavigation } from "./../util"

test("Login takes user to /projects", async ({ baseURL, page }) => {
  await page.goto(baseURL + "/", { waitUntil: "networkidle" })

  // Log-in with the default credentials we set up in the development server
  const signInPage = new SignInPage(baseURL, page)
  await signInPage.submitBuiltInAuthentication(email, password)

  const projectsPage = new ProjectsPage(baseURL, page)
  await waitForClientSideNavigation(page, { to: projectsPage.url })

  await page.waitForSelector("text=Projects")
})
