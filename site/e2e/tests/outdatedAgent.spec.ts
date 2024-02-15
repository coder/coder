import { test } from "@playwright/test";
import { randomUUID } from "crypto";
import {
  createTemplate,
  createWorkspace,
  downloadCoderVersion,
  sshIntoWorkspace,
  startAgentWithCommand,
  stopAgent,
  stopWorkspace,
} from "../helpers";
import { beforeCoderTest } from "../hooks";

// we no longer support versions prior to single tailnet: https://github.com/coder/coder/commit/d7cbdbd9c64ad26821e6b35834c59ecf85dcd9d4
const agentVersion = "v0.27.0";

test.beforeEach(async ({ page }) => await beforeCoderTest(page));

test("ssh with agent " + agentVersion, async ({ page }) => {
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
                  order: 0,
                },
              ],
            },
          ],
        },
      },
    ],
  });
  const workspaceName = await createWorkspace(page, template);
  const binaryPath = await downloadCoderVersion(agentVersion);
  const agent = await startAgentWithCommand(page, token, binaryPath);

  const client = await sshIntoWorkspace(page, workspaceName);
  await new Promise<void>((resolve, reject) => {
    // We just exec a command to be certain the agent is running!
    client.exec("exit 0", (err, stream) => {
      if (err) {
        return reject(err);
      }
      stream.on("exit", (code) => {
        if (code !== 0) {
          return reject(new Error(`Command exited with code ${code}`));
        }
        client.end();
        resolve();
      });
    });
  });

  await stopWorkspace(page, workspaceName);
  await stopAgent(agent, false);
});
