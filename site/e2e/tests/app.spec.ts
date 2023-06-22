import { test } from "@playwright/test"
import { randomUUID } from "crypto"
import * as http from "http"
import { createTemplate, createWorkspace, startAgent } from "../helpers"

test("app", async ({ context, page }) => {
  const appContent = "Hello World"
  const token = randomUUID()
  const srv = http
    .createServer((req, res) => {
      res.writeHead(200, { "Content-Type": "text/plain" })
      res.end(appContent)
    })
    .listen(0)
  const addr = srv.address()
  if (typeof addr !== "object" || !addr) {
    throw new Error("Expected addr to be an object")
  }
  const appName = "test-app"
  const template = await createTemplate(page, {
    apply: [
      {
        complete: {
          resources: [
            {
              agents: [
                {
                  token,
                  apps: [
                    {
                      url: "http://localhost:" + addr.port,
                      displayName: appName,
                    },
                  ],
                },
              ],
            },
          ],
        },
      },
    ],
  })
  await createWorkspace(page, template)
  await startAgent(page, token)

  // Wait for the web terminal to open in a new tab
  const pagePromise = context.waitForEvent("page")
  await page.getByText(appName).click()
  const app = await pagePromise
  await app.waitForLoadState("networkidle")
  await app.getByText(appContent).isVisible()
})
