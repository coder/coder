import { test, expect } from "@playwright/test";
import {
  createGroup,
  createOrganization,
  getCurrentOrgId,
  setupApiCalls,
} from "../api";
import { requiresEnterpriseLicense } from "../helpers";
import { beforeCoderTest } from "../hooks";
import { expectUrl } from "../expectUrl";

test.beforeEach(async ({ page }) => await beforeCoderTest(page));

test("create and delete organization", async ({ page, baseURL }) => {
  requiresEnterpriseLicense();
  await setupApiCalls(page);

  // Create an organzation
  await page.goto(`${baseURL}/organizations/new`, {
    waitUntil: "domcontentloaded",
  });

  await page.getByLabel("Name", { exact: true }).fill("floop");
  await page.getByLabel("Display name").fill("Floop");
  await page.getByLabel("Description").fill("Org description floop");
  await page.getByLabel("Icon", { exact: true }).fill("/emojis/1f957.png");

  await page.getByRole("button", { name: "Submit" }).click();

  // Expect to be redirected to the new organization
  await expectUrl(page).toHavePathName("/organizations/floop");
  await expect(page.getByText("Organization created.")).toBeVisible();

  await page.getByRole("button", { name: "Delete this organization" }).click();
  const dialog = page.getByTestId("dialog");
  await dialog.getByLabel("Name").fill("floop");
  await dialog.getByRole("button", { name: "Delete" }).click();
  await expect(page.getByText("Organization deleted.")).toBeVisible();
});
