import { test } from "@playwright/test";
import {
  createTemplate,
  createWorkspace,
  startAgent,
  stopAgent,
} from "../helpers";
import { randomUUID } from "crypto";
import { beforeCoderTest } from "../hooks";

test.beforeEach(async ({ page }) => await beforeCoderTest(page));

test("web terminal", async ({ context, page }) => {
  const token = randomUUID();
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
  });
  await createWorkspace(page, template);
  const agent = await startAgent(page, token);

  // Wait for the web terminal to open in a new tab
  const pagePromise = context.waitForEvent("page");
  await page.getByTestId("terminal").click();
  const terminal = await pagePromise;
  await terminal.waitForLoadState("domcontentloaded");

  const xtermRows = await terminal.waitForSelector("div.xterm-rows", {
    state: "visible",
  });

  // Ensure that we can type in it
  await terminal.keyboard.type("echo he${justabreak}llo");
  await terminal.keyboard.press("Enter");

  // Check if "echo" command was executed
  await xtermRows.waitForSelector('div:text-matches("hello")', {
    state: "visible",
  });

  await stopAgent(agent);
});
