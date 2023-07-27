import { test } from "@playwright/test"
import { createTemplate, createWorkspace, downloadCoderVersion, findSessionToken, startAgentWithCommand } from "../helpers"
import { randomUUID } from "crypto"
import { spawn } from "child_process"
import path from "path"

test("create workspace with an outdated agent", async ({ page }, testInfo) => {
  const token = randomUUID()
  const template = await createTemplate(page, {
    apply: [
      {
        complete: {
          resources: [
            {
              agents: [
                {
                  token,
                },
              ],
            },
          ],
        },
      },
    ],
  })
  const workspace = await createWorkspace(page, template)
  const binaryPath = await downloadCoderVersion(testInfo, "v0.24.0")
  await startAgentWithCommand(page, token, binaryPath)
  const sessionToken = await findSessionToken(page)
  const coderMain = path.join(
    __dirname,
    "..",
    "..",
    "..",
    "enterprise",
    "cmd",
    "coder",
    "main.go",
  )
  await new Promise<void>((resolve, reject) => {
    const cp = spawn("ssh", [
      "-o", "StrictHostKeyChecking=no",
      "-o", "UserKnownHostsFile=/dev/null",
      "-o", "ProxyCommand=/usr/local/go/bin/go run "+coderMain+" ssh --stdio " + workspace,
      "localhost",
      "exit",
      "0",
    ], {
      env: {
        ...process.env,
        CODER_SESSION_TOKEN: sessionToken,
        CODER_URL: "http://localhost:3000",
      },
    })
    cp.stderr.on("data", (data) => console.log(data.toString()))
    cp.stdout.on("data", (data) => console.log(data.toString()))
    cp.on("close", (code) => {
      if (code === 0) {
        resolve()
      } else {
        reject(new Error("ssh failed with code " + code))
      }
    })
  })
})
