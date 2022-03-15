import * as path from "path"
import { PlaywrightTestConfig } from "@playwright/test"

const config: PlaywrightTestConfig = {
  testDir: "tests",
  globalSetup: require.resolve("./globalSetup"),

  // Create junit report file for upload to DataDog
  reporter: [["junit", { outputFile: "test-results/junit.xml" }]],

  use: {
    baseURL: "http://localhost:3000",
    video: "retain-on-failure",
  },

  // `webServer` tells Playwright to launch a test server - more details here:
  // https://playwright.dev/docs/test-advanced#launching-a-development-web-server-during-the-tests
  webServer: {
    // Run the coder daemon directly.
    command: `go run ${path.join(__dirname, "../../cmd/coder/main.go")} daemon`,
    port: 3000,
    timeout: 120 * 10000,
    reuseExistingServer: false,
  },
}

export default config
