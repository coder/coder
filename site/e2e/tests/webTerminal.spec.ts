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

  await terminal.waitForSelector("div.xterm-rows", {
    state: "visible",
  });

  // Workaround: delay next steps as "div.xterm-rows" can be recreated/reattached
  // after a couple of milliseconds.
  await terminal.waitForTimeout(2000);

  // Ensure that we can type in it
  await terminal.keyboard.type("echo he${justabreak}llo123456");
  await terminal.keyboard.press("Enter");

  // Check if "echo" command was executed
  // try-catch is used temporarily to find the root cause: https://github.com/coder/coder/actions/runs/6176958762/job/16767089943
  try {
    await terminal.waitForSelector(
      'div.xterm-rows div:text-matches("hello123456")',
      {
        state: "visible",
        timeout: 10 * 1000,
      },
    );
  } catch (error) {
    const pageContent = await terminal.content();
    // eslint-disable-next-line no-console -- Let's see what is inside of xterm-rows
    console.log("Unable to find echoed text:", pageContent);
    throw error;
  }

  await stopAgent(agent);
});
