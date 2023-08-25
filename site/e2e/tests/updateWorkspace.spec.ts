import { test } from "@playwright/test"
import {
  createTemplate,
  createWorkspace,
  echoResponsesWithParameters,
  uploadTemplateVersion,
  verifyParameters,
} from "../helpers"

import { fifthParameter, firstParameter, secondParameter } from "../parameters"
import { RichParameter } from "../provisionerGenerated"

test("update workspace, new optional, mutable parameter added", async ({
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

  // Upload new template version with extra parameter.
  const updatedRichParameters = [...richParameters, fifthParameter]
  const templateVersion = await uploadTemplateVersion(
    template,
    echoResponsesWithParameters(updatedRichParameters),
  )

  // TODO Activate the template version
  // Go to Versions -> Promote version

  // Now, update the workspace.
  // TODO Update workspace

  // Verify that parameter values are default.
  await verifyParameters(page, workspaceName, updatedRichParameters, [
    { name: firstParameter.name, value: firstParameter.defaultValue },
    { name: secondParameter.name, value: secondParameter.defaultValue },
    { name: fifthParameter.name, value: fifthParameter.defaultValue },
  ])
})
