import { test, expect } from "@playwright/test"
import { getStatePath } from "../helpers"

test.use({ storageState: getStatePath("authState") })

test("signing out redirects to login page", async ({ page, baseURL }) => {
  await page.goto(`${baseURL}/`, { waitUntil: "networkidle" })

  await page.getByTestId("user-dropdown-trigger").click()
  await page.getByRole("menuitem", { name: "Sign Out" }).click()

  await expect(
    page.getByRole("heading", { name: "Sign in to Coder" }),
  ).toBeVisible()

  expect(page.url()).toMatch(/\/login$/) // ensure we're on the login page with no query params
})
