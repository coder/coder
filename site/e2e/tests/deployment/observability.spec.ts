import { test } from "@playwright/test";
import { API } from "api/api";
import {
  setupApiCalls,
  verifyConfigFlagArray,
  verifyConfigFlagBoolean,
  verifyConfigFlagDuration,
  verifyConfigFlagEmpty,
  verifyConfigFlagString,
} from "../../api";

test("enabled observability settings", async ({ page }) => {
  await setupApiCalls(page);
  const config = await API.getDeploymentConfig();

  await page.goto("/deployment/observability", {
    waitUntil: "domcontentloaded",
  });

  await verifyConfigFlagBoolean(page, config, "trace-logs");
  await verifyConfigFlagBoolean(page, config, "enable-terraform-debug-mode");
  await verifyConfigFlagBoolean(page, config, "enable-terraform-debug-mode");
  await verifyConfigFlagDuration(page, config, "health-check-refresh");
  await verifyConfigFlagEmpty(page, "health-check-threshold-database");
  await verifyConfigFlagString(page, config, "log-human");
  await verifyConfigFlagString(page, config, "prometheus-address");
  await verifyConfigFlagArray(
    page,
    config,
    "prometheus-aggregate-agent-stats-by",
  );
  await verifyConfigFlagBoolean(page, config, "prometheus-collect-agent-stats");
  await verifyConfigFlagBoolean(page, config, "prometheus-collect-db-metrics");
  await verifyConfigFlagBoolean(page, config, "prometheus-enable");
  await verifyConfigFlagBoolean(page, config, "trace-datadog");
  await verifyConfigFlagBoolean(page, config, "trace");
  await verifyConfigFlagBoolean(page, config, "verbose");
  await verifyConfigFlagBoolean(page, config, "pprof-enable");
});
