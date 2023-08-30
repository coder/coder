import { test } from "@playwright/test"
import {
  buildWorkspaceWithParameters,
  createTemplate,
  createWorkspace,
  echoResponsesWithParameters,
  verifyParameters,
} from "../helpers"

import { firstBuildOption, secondBuildOption } from "../parameters"
import { RichParameter } from "../provisionerGenerated"
import { beforeCoderTest } from "../hooks"

test.beforeEach(async ({ page }) => await beforeCoderTest(page))

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
  await buildWorkspaceWithParameters(
    page,
    workspaceName,
    richParameters,
    buildParameters,
    true,
  )

  // Verify that build options are default (not selected).
  await verifyParameters(page, workspaceName, richParameters, [
    { name: firstBuildOption.name, value: firstBuildOption.defaultValue },
    { name: secondBuildOption.name, value: secondBuildOption.defaultValue },
  ])
})
