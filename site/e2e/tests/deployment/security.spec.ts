import { expect, test, type Page } from "@playwright/test";
import * as API from "api/api";
import { setupApiCalls } from "../../api";

test("enabled security settings", async ({ page }) => {
  await setupApiCalls(page);

  const config = await API.getDeploymentConfig();

  await page.goto("/deployment/security", { waitUntil: "domcontentloaded" });

  const flags = [
    "ssh-keygen-algorithm",
    "secure-auth-cookie",
    "disable-owner-workspace-access",

    "tls-redirect-http-to-https",
    "strict-transport-security",
    "tls-address",
    "tls-allow-insecure-ciphers",
    "tls-client-auth",
    "tls-enable",
    "tls-min-version",
  ];

  for (const flag of flags) {
    await verifyConfigFlag(page, config, flag);
  }
});

const verifyConfigFlag = async (
  page: Page,
  config: API.DeploymentConfig,
  flag: string,
) => {
  const opt = config.options.find((option) => option.flag === flag);
  if (opt === undefined) {
    throw new Error(`Option with env ${flag} has undefined value.`);
  }

  // Map option type to test class name.
  let type = "",
    value = opt.value;
  if (typeof value === "boolean") {
    // Boolean options map to string (Enabled/Disabled).
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

  // Special cases
  if (opt.flag === "strict-transport-security" && opt.value === 0) {
    type = "option-value-string";
    value = "Disabled"; // Display "Disabled" instead of zero seconds.
  }

  const configOption = page.locator(
    `div.options-table .option-${flag} .${type}`,
  );
  await expect(configOption).toHaveText(String(value));
};
