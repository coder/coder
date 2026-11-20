import * as path from "node:path";
import { defineConfig, devices } from "@playwright/test";

// Previews need their own coderd: a wildcard app hostname that is same-site
// with the dashboard, and an access URL that names the dashboard origin the
// preview CSP allows as a frame ancestor. The browsers resolve *.localhost
// to the loopback address, and Chromium and Firefox treat it as a secure
// context, so they send the Fetch Metadata headers that preview hosts
// require.
const previewsPort = 3121;
const dashboardHost = "coder.localhost";

// The shared e2e helpers read the coderd port from this variable, and
// Playwright workers inherit the runner's environment.
process.env.CODER_E2E_PORT = String(previewsPort);

// Run with `pnpm playwright:test-previews`. Firefox and WebKit need
// `pnpm playwright install --with-deps firefox webkit` once.
export default defineConfig({
	outputDir: "../test-results/previews",
	// Each test starts a workspace agent, and the e2e agent helper binds fixed
	// pprof and Prometheus ports, so tests run one at a time.
	workers: 1,
	projects: [
		{
			name: "previewsSetup",
			testMatch: /setup\/addUsersAndLicense\.spec\.ts/,
		},
		...[
			{ browser: "chromium", use: devices["Desktop Chrome"] },
			{
				browser: "firefox",
				use: {
					...devices["Desktop Firefox"],
					// Firefox sends link pings only when this is on, and the
					// test needs every vector to be live.
					launchOptions: { firefoxUserPrefs: { "browser.send_pings": true } },
				},
			},
			{ browser: "webkit", use: devices["Desktop Safari"] },
		].map(({ browser, use }) => ({
			name: `previews-${browser}`,
			testMatch: /previews\/.*\.spec\.ts/,
			dependencies: ["previewsSetup"],
			timeout: 120_000,
			use,
		})),
	],
	reporter: [
		["list"],
		["html", { open: "never", outputFolder: "../playwright-report/previews" }],
	],
	use: {
		actionTimeout: 5000,
		baseURL: `http://${dashboardHost}:${previewsPort}`,
		screenshot: "only-on-failure",
		trace: "retain-on-failure",
	},
	webServer: {
		url: `http://127.0.0.1:${previewsPort}/healthz`,
		timeout: 120_000,
		command: [
			`"${path.join(__dirname, "bin/coder")}"`,
			"server",
			"--global-config $(mktemp -d -t e2e-previews-XXXXXXXXXX)",
			`--access-url=http://${dashboardHost}:${previewsPort}`,
			// The app sign-in redirect builds app hosts from the wildcard as
			// written, so it carries the port.
			`--wildcard-access-url=*.${dashboardHost}:${previewsPort}`,
			`--http-address=127.0.0.1:${previewsPort}`,
			"--ephemeral",
			"--telemetry=false",
			"--dangerous-disable-rate-limits",
			"--provisioner-types=echo",
			"--provisioner-daemons=3",
			"--log-filter=.*",
			`--log-human=${path.join(__dirname, "test-results/previews-debug.log")}`,
		].join(" "),
		stdout: "pipe",
		env: {
			...process.env,
			CGO_ENABLED: "0",
			CODER_EXPERIMENTS: "open-preview",
		},
		reuseExistingServer: false,
	},
});
