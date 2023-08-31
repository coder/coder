import { test, expect } from "@playwright/test"

test("list templates", async ({ page, baseURL }) => {
  await page.goto(`${baseURL}/templates`, { waitUntil: "domcontentloaded" })
  await expect(page).toHaveTitle("Templates - Coder")
})
