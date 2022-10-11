import { PlaywrightTestConfig } from "@playwright/test"
import * as path from "path"
import { basePort } from "./constants"

const config: PlaywrightTestConfig = {
  testDir: "tests",
  globalSetup: require.resolve("./globalSetup"),

  // Create junit report file for upload to DataDog
  reporter: [["junit", { outputFile: "test-results/junit.xml" }]],

  // NOTE: if Playwright complains about the port being taken
  // do not change the basePort (it must match our api server).
  // Instead, simply run the test suite without running our local server.
  use: {
    baseURL: `http://localhost:${basePort}`,
    video: "retain-on-failure",
  },

  // `webServer` tells Playwright to launch a test server - more details here:
  // https://playwright.dev/docs/test-advanced#launching-a-development-web-server-during-the-tests
  webServer: {
    // Run the coder daemon directly.
    command: `go run -tags embed ${path.join(
      __dirname,
      "../../enterprise/cmd/coder/main.go",
    )} server --in-memory --access-url 127.0.0.1:${basePort}`,
    port: basePort,
    timeout: 120 * 10000,
    reuseExistingServer: false,
  },
}

export default config
