import { randomUUID } from "crypto";
import * as http from "http";
import {
  createTemplate,
  createWorkspace,
  startAgent,
  stopAgent,
  stopWorkspace,
} from "../helpers";
import { beforeCoderTest } from "../hooks";
import { test } from "../testing";

test.beforeEach(({ page }) => beforeCoderTest(page));

const appName = "test-app";
const appContent = "Hello World";

test("app", async ({ context, page }) => {
  console.log("what the heck");

  const token = randomUUID();
  const srv = http
    .createServer((req, res) => {
      res.writeHead(200, { "Content-Type": "text/plain" });
      res.end(appContent);
    })
    .listen(0);
  const addr = srv.address();
  if (typeof addr !== "object" || !addr) {
    throw new Error("Expected addr to be an object");
  }

  console.log("marker: a");
  const template = await createTemplate(
    page,
    echoProvisionerResponses(addr.port, token),
  );
  console.log("marker: b");
  const workspaceName = await createWorkspace(page, template);
  console.log("marker: c");
  const agent = await startAgent(page, token);

  // Wait for the web terminal to open in a new tab
  console.log("marker: d");
  const pagePromise = context.waitForEvent("page");
  console.log("marker: e");
  await page.getByText(appName).click();
  console.log("marker: f");
  const app = await pagePromise;
  console.log("marker: g");
  await app.waitForLoadState("domcontentloaded");
  console.log("marker: h");
  await app.getByText(appContent).isVisible();

  console.log("marker: i");
  await stopWorkspace(page, workspaceName);
  console.log("marker: j");
  await stopAgent(agent);
  console.log("marker: k");
});

const echoProvisionerResponses = (port: number, token: string) => ({
  apply: [
    {
      apply: {
        resources: [
          {
            agents: [
              {
                token,
                apps: [
                  {
                    url: "http://localhost:" + port,
                    displayName: appName,
                    order: 0,
                  },
                ],
                order: 0,
              },
            ],
          },
        ],
      },
    },
  ],
});
