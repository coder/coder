import { PlaywrightTestConfig } from "@playwright/test"
import path from "path"
import { defaultPort } from "./constants"

const port = process.env.CODER_E2E_PORT
  ? Number(process.env.CODER_E2E_PORT)
  : defaultPort

const coderMain = path.join(__dirname, "../../enterprise/cmd/coder/main.go")

const config: PlaywrightTestConfig = {
  testDir: "tests",
  globalSetup: require.resolve("./globalSetup"),
  use: {
    baseURL: `http://localhost:${port}`,
    video: "retain-on-failure",
  },
  webServer: {
    command: `go run -tags embed ${coderMain} server --global-config $(mktemp -d -t e2e-XXXXXXXXXX)`,
    port,
    reuseExistingServer: false,
  },
}

export default config
