import { expect, test } from "@playwright/test";
import * as API from "api/api";
import { setupApiCalls } from "../../api";

test("experiments", async ({ page }) => {
  await setupApiCalls(page);

  // Load experiments from backend API
  const availableExperiments = await API.getAvailableExperiments();
  const enabledExperiments = await API.getExperiments();

  // Verify if the site lists the same experiments
  await page.goto("/deployment/general", { waitUntil: "networkidle" });

  const experimentsLocator = page.locator(
    "div.options-table tr.option-experiments ul.option-array",
  );
  await expect(experimentsLocator).toBeVisible();

  await experimentsLocator.focus();
  await page.mouse.wheel(0, 600);

  // eslint-disable-next-line no-console -- HTML for experiments
  console.log(experimentsLocator.innerHTML());

  // Firstly, check if available experiments are listed
  availableExperiments.safe.map(async (experiment) => {
    const experimentLocator = experimentsLocator.locator(
      `li.option-array-item-${experiment}`,
    );
    await expect(experimentLocator).toBeVisible();
  });

  // Secondly, check if all enabled experiments are listed
  enabledExperiments.map(async (experiment) => {
    const experimentLocator = experimentsLocator.locator(
      `li.option-array-item-${experiment}.option-enabled`,
    );
    await expect(experimentLocator).toBeVisible();
  });
});
