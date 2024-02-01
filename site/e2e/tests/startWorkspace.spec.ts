import { test } from "@playwright/test";
import {
  buildWorkspaceWithParameters,
  createTemplate,
  createWorkspace,
  echoResponsesWithParameters,
  stopWorkspace,
  verifyParameters,
} from "../helpers";

import { firstBuildOption, secondBuildOption } from "../parameters";
import { RichParameter } from "../provisionerGenerated";

test("start workspace with ephemeral parameters", async ({ page }) => {
  const richParameters: RichParameter[] = [firstBuildOption, secondBuildOption];
  const template = await createTemplate(
    page,
    echoResponsesWithParameters(richParameters),
  );
  const workspaceName = await createWorkspace(page, template);

  // Verify that build options are default (not selected).
  await verifyParameters(page, workspaceName, richParameters, [
    { name: richParameters[0].name, value: firstBuildOption.defaultValue },
    { name: richParameters[1].name, value: secondBuildOption.defaultValue },
  ]);

  // Stop the workspace
  await stopWorkspace(page, workspaceName);

  // Now, start the workspace with ephemeral parameters selected.
  const buildParameters = [
    { name: richParameters[0].name, value: "AAAAA" },
    { name: richParameters[1].name, value: "true" },
  ];

  await buildWorkspaceWithParameters(
    page,
    workspaceName,
    richParameters,
    buildParameters,
  );

  // Verify that build options are default (not selected).
  await verifyParameters(page, workspaceName, richParameters, [
    { name: richParameters[0].name, value: firstBuildOption.defaultValue },
    { name: richParameters[1].name, value: secondBuildOption.defaultValue },
  ]);
});
