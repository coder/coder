import { test } from "@playwright/test"
import { email, password } from "../constants"
import { SignInPage } from "../pom"
import { clickButtonByText, buttons, urls } from "../helpers";

test("Basic flow", async ({ baseURL, page }) => {
  await page.goto(baseURL + "/", { waitUntil: "networkidle" })

  // Log-in with the default credentials we set up in the development server
  const signInPage = new SignInPage(baseURL, page)
  await signInPage.submitBuiltInAuthentication(email, password)

  await page.waitForSelector("text=Workspaces")

  // create Docker template
  await page.goto(urls.templates);
  await clickButtonByText(page, buttons.starterTemplates)

  await page.goto(urls.starterTemplates);
  await clickButtonByText(page, buttons.dockerTemplate)

  await page.goto(urls.dockerTemplate);
  await clickButtonByText(page, buttons.useTemplate)

  await page.goto(urls.createDockerTemplate);
  await clickButtonByText(page, buttons.createTemplate)
})
