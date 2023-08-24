import { test } from "@playwright/test"
import {
  createTemplate,
  createWorkspace,
  echoResponsesWithParameters,
  restartWorkspace,
  verifyParameters,
} from "../helpers"

import { firstBuildOption, secondBuildOption } from "../parameters"
import { RichParameter } from "../provisionerGenerated"

test("restart workspace with ephemeral parameters", async ({ page }) => {
  const richParameters: RichParameter[] = [firstBuildOption, secondBuildOption]
  const template = await createTemplate(
    page,
    echoResponsesWithParameters(richParameters),
  )
  const workspaceName = await createWorkspace(page, template)

  // Verify that build options are default (not selected).
  await verifyParameters(page, workspaceName, richParameters, [
    { name: firstBuildOption.name, value: firstBuildOption.defaultValue },
    { name: secondBuildOption.name, value: secondBuildOption.defaultValue },
  ])

  // Now, restart the workspace with ephemeral parameters selected.
  const buildParameters = [
    { name: firstBuildOption.name, value: "AAAAA" },
    { name: secondBuildOption.name, value: "true" },
  ]
  await restartWorkspace(page, workspaceName, richParameters, buildParameters)

  // Verify that build options are default (not selected).
  await verifyParameters(page, workspaceName, richParameters, [
    { name: firstBuildOption.name, value: buildParameters[0].value },
    { name: secondBuildOption.name, value: buildParameters[1].value },
  ])
})
