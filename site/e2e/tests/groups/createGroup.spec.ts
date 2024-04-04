import { test, expect } from "@playwright/test";
import { randomName, requiresEnterpriseLicense } from "../../helpers";
import { beforeCoderTest } from "../../hooks";

test.beforeEach(async ({ page }) => await beforeCoderTest(page));

test("create group", async ({ page, baseURL }) => {
  requiresEnterpriseLicense();
  await page.goto(`${baseURL}/groups`, { waitUntil: "domcontentloaded" });
  await expect(page).toHaveTitle("Groups - Coder");

  await page.getByText("Create group").click();
  await expect(page).toHaveTitle("Create Group - Coder");

  const name = randomName();
  const groupValues = {
    name: name,
    displayName: `Display Name for ${name}`,
    avatarURL: "/emojis/1f60d.png",
  };

  await page.getByLabel("Name", { exact: true }).fill(groupValues.name);
  await page.getByLabel("Display Name").fill(groupValues.displayName);
  await page.getByLabel("Avatar URL").fill(groupValues.avatarURL);
  await page.getByRole("button", { name: "Submit" }).click();

  await expect(page).toHaveTitle(`${groupValues.displayName} - Coder`);
  await expect(page.getByText(groupValues.displayName)).toBeVisible();
  await expect(page.getByText("No members yet")).toBeVisible();
});
