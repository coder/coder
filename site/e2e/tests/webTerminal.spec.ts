import { expect, test } from "@playwright/test"
import { createTemplate, createWorkspace, startAgent } from "../helpers"
import { randomUUID } from "crypto"

test("web terminal", async ({ context, page }) => {
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

  // Make sure the text came back
  const number = await terminal.locator("text=hello").all()
  expect(number.length).toBe(2)
})
