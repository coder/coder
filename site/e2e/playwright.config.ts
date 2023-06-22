import { defineConfig } from "@playwright/test"
import path from "path"
import { defaultPort } from "./constants"

export const port = process.env.CODER_E2E_PORT
  ? Number(process.env.CODER_E2E_PORT)
  : defaultPort

const coderMain = path.join(__dirname, "../../enterprise/cmd/coder/main.go")

export const STORAGE_STATE = path.join(__dirname, ".auth.json")

const config = defineConfig({
  projects: [
    {
      name: "setup",
      testMatch: /global.setup\.ts/,
    },
    {
      name: "tests",
      testMatch: /.*\.spec\.ts/,
      dependencies: ["setup"],
      use: {
        storageState: STORAGE_STATE,
      },
    },
  ],
  use: {
    baseURL: `http://localhost:${port}`,
    video: "retain-on-failure",
  },
  webServer: {
    command:
      `go run -tags embed ${coderMain} server ` +
      `--global-config $(mktemp -d -t e2e-XXXXXXXXXX) ` +
      `--access-url=http://localhost:${port} ` +
      `--http-address=localhost:${port} ` +
      `--in-memory --telemetry=false ` +
      `--provisioner-daemons 10 ` +
      `--provisioner-daemons-echo ` +
      `--provisioner-daemon-poll-interval 50ms`,
    port,
    reuseExistingServer: false,
  },
})

export default config
