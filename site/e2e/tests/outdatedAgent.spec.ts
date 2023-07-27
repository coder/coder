import { test } from "@playwright/test"
import { randomUUID } from "crypto"
import {
  createTemplate,
  createWorkspace,
  downloadCoderVersion,
  sshIntoWorkspace,
  startAgentWithCommand,
} from "../helpers"

const agentVersion = "v0.14.0"

test("ssh with agent " + agentVersion, async ({ page }) => {
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
  const binaryPath = await downloadCoderVersion(agentVersion)
  await startAgentWithCommand(page, token, binaryPath)

  const client = await sshIntoWorkspace(page, workspace)
  await new Promise<void>((resolve, reject) => {
    // We just exec a command to be certain the agent is running!
    client.exec("exit 0", (err, stream) => {
      if (err) {
        return reject(err)
      }
      stream.on("exit", (code) => {
        if (code !== 0) {
          return reject(new Error(`Command exited with code ${code}`))
        }
        client.end()
        resolve()
      })
    })
  })
})
