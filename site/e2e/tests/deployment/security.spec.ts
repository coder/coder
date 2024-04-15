import { expect, test, type Page } from "@playwright/test";
import * as API from "api/api";
import { setupApiCalls } from "../../api";

test("enabled security settings", async ({ page }) => {
  await setupApiCalls(page);

  await page.goto("/deployment/security", { waitUntil: "domcontentloaded" });

  // Load deployment settings
  const config = await API.getDeploymentConfig();

  // Check flags
  await expectConfigOption(page, config, "ssh-keygen-algorithm");
  await expectConfigOption(page, config, "secure-auth-cookie");
  await expectConfigOption(page, config, "disable-owner-workspace-access");

  await expectConfigOption(page, config, "tls-redirect-http-to-https");
  await expectConfigOption(page, config, "strict-transport-security");
  await expectConfigOption(page, config, "tls-address");
  await expectConfigOption(page, config, "tls-allow-insecure-ciphers");
});

const expectConfigOption = async (
  page: Page,
  config: API.DeploymentConfig,
  flag: string,
) => {
  let value = config.options.find((option) => option.flag === flag)?.value;
  if (value === undefined) {
    throw new Error(`Option with env ${flag} has undefined value.`);
  }

  let type = "";
  if (typeof value === "boolean") {
    type = value ? "option-enabled" : "option-disabled";
    value = value ? "Enabled" : "Disabled";
  } else if (typeof value === "number") {
    type = "option-value-number";
    value = String(value);
  } else if (!value || value.length === 0) {
    type = "option-value-empty";
  } else if (typeof value === "string") {
    type = "option-value-string";
  } else if (typeof value === "object") {
    type = "object-array";
  } else {
    type = "option-value-json";
  }

  const configOption = page.locator(
    `div.options-table .option-${flag} .${type}`,
  );
  await expect(configOption).toHaveText(String(value));
};
