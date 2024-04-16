import { test } from "@playwright/test";
import { getDeploymentConfig } from "api/api";
import { setupApiCalls, verifyConfigFlag } from "../../api";

test("enabled security settings", async ({ page }) => {
  await setupApiCalls(page);
  const config = await getDeploymentConfig();

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
