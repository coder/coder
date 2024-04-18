import type { Page } from "@playwright/test";
import { expect, test } from "@playwright/test";
import type * as API from "api/api";
import { getDeploymentConfig } from "api/api";
import {
  findConfigOption,
  setupApiCalls,
  verifyConfigFlagBoolean,
  verifyConfigFlagNumber,
  verifyConfigFlagString,
} from "../../api";

test("enabled security settings", async ({ page }) => {
  await setupApiCalls(page);
  const config = await getDeploymentConfig();

  await page.goto("/deployment/security", { waitUntil: "domcontentloaded" });

  await verifyConfigFlagString(page, config, "ssh-keygen-algorithm");
  await verifyConfigFlagBoolean(page, config, "secure-auth-cookie");
  await verifyConfigFlagBoolean(page, config, "disable-owner-workspace-access");

  await verifyConfigFlagBoolean(page, config, "tls-redirect-http-to-https");
  await verifyStrictTransportSecurity(page, config);
  await verifyConfigFlagString(page, config, "tls-address");
  await verifyConfigFlagBoolean(page, config, "tls-allow-insecure-ciphers");
  await verifyConfigFlagString(page, config, "tls-client-auth");
  await verifyConfigFlagBoolean(page, config, "tls-enable");
  await verifyConfigFlagString(page, config, "tls-min-version");
});

async function verifyStrictTransportSecurity(
  page: Page,
  config: API.DeploymentConfig,
) {
  const flag = "strict-transport-security";
  const opt = findConfigOption(config, flag);
  if (opt.value !== 0) {
    await verifyConfigFlagNumber(page, config, flag);
    return;
  }

  const configOption = page.locator(
    `div.options-table .option-${flag} .option-value-string`,
  );
  await expect(configOption).toHaveText("Disabled");
}
