import { test } from "@playwright/test"
import { createTemplate, createWorkspace, startAgent } from "../helpers"
import { randomUUID } from "crypto"
import { beforeCoderTest } from "../hooks"

test.beforeEach(async ({ page }) => await beforeCoderTest(page))

test("web terminal", async ({ context, page }) => {
  const token = randomUUID()
  const template = await createTemplate(page, {
    apply: [
      {
        apply: {
          resources: [
            {
              agents: [
                {
                  token,
                  displayApps: {
                    webTerminal: true,
                  },
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
  await page.getByTestId("terminal").click()
  const terminal = await pagePromise
  await terminal.waitForLoadState("networkidle")

  // Ensure that we can type in it
  await terminal.keyboard.type("echo hello")
  await terminal.keyboard.press("Enter")

  const locator = terminal.locator("text=hello")

  for (let i = 0; i < 10; i++) {
    const items = await locator.all()
    // Make sure the text came back
    if (items.length === 2) {
      break
    }
    await new Promise((r) => setTimeout(r, 250))
  }
})
