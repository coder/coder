import { test } from "@playwright/test"
import {
  createTemplate,
  createWorkspace,
  echoResponsesWithParameters,
  updateTemplate,
  updateWorkspace,
  verifyParameters,
} from "../helpers"

import { fifthParameter, firstParameter, secondParameter } from "../parameters"
import { RichParameter } from "../provisionerGenerated"

test("update workspace, new optional, immutable parameter added", async ({
  page,
}) => {
  const richParameters: RichParameter[] = [firstParameter, secondParameter]
  const template = await createTemplate(
    page,
    echoResponsesWithParameters(richParameters),
  )

  const workspaceName = await createWorkspace(page, template)

  // Verify that parameter values are default.
  await verifyParameters(page, workspaceName, richParameters, [
    { name: firstParameter.name, value: firstParameter.defaultValue },
    { name: secondParameter.name, value: secondParameter.defaultValue },
  ])

  // Push updated template.
  const updatedRichParameters = [...richParameters, fifthParameter]
  await updateTemplate(
    page,
    template,
    echoResponsesWithParameters(updatedRichParameters),
  )

  // Now, update the workspace, and select the value for immutable parameter.
  await updateWorkspace(page, workspaceName, updatedRichParameters, [
    { name: fifthParameter.name, value: fifthParameter.options[0].value },
  ])

  // Verify that parameter values are default.
  await verifyParameters(page, workspaceName, updatedRichParameters, [
    { name: firstParameter.name, value: firstParameter.defaultValue },
    { name: secondParameter.name, value: secondParameter.defaultValue },
    { name: fifthParameter.name, value: fifthParameter.options[0].value },
  ])
})
