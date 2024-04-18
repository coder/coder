import { test } from "@playwright/test";
import { getDeploymentConfig } from "api/api";
import {
  setupApiCalls,
  verifyConfigFlagArray,
  verifyConfigFlagBoolean,
  verifyConfigFlagDuration,
  verifyConfigFlagNumber,
  verifyConfigFlagString,
} from "../../api";

test("enabled network settings", async ({ page }) => {
  await setupApiCalls(page);
  const config = await getDeploymentConfig();

  await page.goto("/deployment/network", { waitUntil: "domcontentloaded" });

  await verifyConfigFlagString(page, config, "access-url");
  await verifyConfigFlagBoolean(page, config, "block-direct-connections");
  await verifyConfigFlagBoolean(page, config, "browser-only");
  await verifyConfigFlagBoolean(page, config, "derp-force-websockets");
  await verifyConfigFlagBoolean(page, config, "derp-server-enable");
  await verifyConfigFlagString(page, config, "derp-server-region-code");
  await verifyConfigFlagString(page, config, "derp-server-region-code");
  await verifyConfigFlagNumber(page, config, "derp-server-region-id");
  await verifyConfigFlagString(page, config, "derp-server-region-name");
  await verifyConfigFlagArray(page, config, "derp-server-stun-addresses");
  await verifyConfigFlagBoolean(page, config, "disable-password-auth");
  await verifyConfigFlagBoolean(page, config, "disable-session-expiry-refresh");
  await verifyConfigFlagDuration(page, config, "max-token-lifetime");
  await verifyConfigFlagDuration(page, config, "proxy-health-interval");
  await verifyConfigFlagBoolean(page, config, "redirect-to-access-url");
  await verifyConfigFlagBoolean(page, config, "secure-auth-cookie");
  await verifyConfigFlagDuration(page, config, "session-duration");
  await verifyConfigFlagString(page, config, "tls-address");
  await verifyConfigFlagBoolean(page, config, "tls-allow-insecure-ciphers");
  await verifyConfigFlagString(page, config, "tls-client-auth");
  await verifyConfigFlagBoolean(page, config, "tls-enable");
  await verifyConfigFlagString(page, config, "tls-min-version");
});
