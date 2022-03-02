import { test } from "@playwright/test"
import { SignInPage } from "../pom"
import { email, password } from "../constants"
import ProjectPage from "../../pages/projects/[organization]/[project]"

test("Login takes user to /projects", async ({ baseURL, page }) => {
  await page.goto(baseURL + "/", { waitUntil: "networkidle" })

  // Log-in with the default credentials we set up in the development server
  const signInPage = new SignInPage(baseURL, page)
  await signInPage.submitBuiltInAuthentication(email, password)

  const projectsPage = new ProjectPage(baseURL, page)
  await page.waitForNavigation({ url: projectsPage.url, waitUntil: "networkidle" })

  await page.waitForSelector("text=Projects")
})
