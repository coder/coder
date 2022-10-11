import { test } from "@playwright/test"
import { HealthzPage } from "../pom/HealthzPage"

test("Healthz is available without authentication", async ({
  baseURL,
  page,
}) => {
  const healthzPage = new HealthzPage(baseURL, page)
  await page.goto(healthzPage.url, { waitUntil: "networkidle" })
  await healthzPage.getOk().waitFor({ state: "visible" })
})
