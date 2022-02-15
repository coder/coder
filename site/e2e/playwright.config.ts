import * as path from "path"
import { PlaywrightTestConfig } from "@playwright/test"

const config: PlaywrightTestConfig = {
  testDir: "tests",

  use: {
    video: "retain-on-failure",
  },

  // `webServer` tells Playwright to launch a test server - more details here:
  // https://playwright.dev/docs/test-advanced#launching-a-development-web-server-during-the-tests
  webServer: {
    command: path.join(__dirname, "../../develop.sh"),
    port: 8080,
    timeout: 120 * 10000,
    reuseExistingServer: false,
    env: {
      HUMAN_LOG: "1",
    },
  },
}

export default config
