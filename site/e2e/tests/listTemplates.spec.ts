import { test, expect } from "@playwright/test"

test("list templates", async ({ page, baseURL }) => {
  await page.goto(`${baseURL}/templates`, { waitUntil: "networkidle" })
  await expect(page).toHaveTitle("Templates - Coder")
})
