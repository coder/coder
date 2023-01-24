import { test, expect } from "@playwright/test"
import { getStatePath } from "../helpers"

test.use({ storageState: getStatePath("authState") })

test("list templates", async ({ page, baseURL }) => {
  await page.goto(`${baseURL}/templates`, { waitUntil: "networkidle" })
  await expect(page).toHaveTitle("Templates – Coder")
})
