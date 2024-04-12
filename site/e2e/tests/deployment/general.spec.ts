import { expect, test } from "@playwright/test";
import * as API from "api/api";
import { setupApiCalls } from "../../api";
import { e2eFakeExperiment1, e2eFakeExperiment2 } from "../../constants";

test("experiments", async ({ page }) => {
  await setupApiCalls(page);

  // Load experiments from backend API
  const availableExperiments = await API.getAvailableExperiments();

  // Verify if the site lists the same experiments
  await page.goto("/deployment/general", { waitUntil: "networkidle" });

  const experimentsLocator = page.locator(
    "div.options-table tr.option-experiments ul.option-array",
  );
  await expect(experimentsLocator).toBeVisible();

  // Firstly, check if all enabled experiments are listed
  expect(
    experimentsLocator.locator(
      `li.option-array-item-${e2eFakeExperiment1}.option-enabled`,
    ),
  ).toBeVisible;
  expect(
    experimentsLocator.locator(
      `li.option-array-item-${e2eFakeExperiment2}.option-enabled`,
    ),
  ).toBeVisible;

  // Secondly, check if available experiments are listed
  for (const experiment of availableExperiments.safe) {
    const experimentLocator = experimentsLocator.locator(
      `li.option-array-item-${experiment}`,
    );
    await expect(experimentLocator).toBeVisible();
  }
});
