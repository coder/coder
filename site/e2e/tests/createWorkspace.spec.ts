import { test } from "@playwright/test"
import {
  createTemplate,
  createWorkspace,
  echoResponsesWithParameters,
  verifyParameters,
} from "../helpers"

import {
  secondParameter,
  fourthParameter,
  fifthParameter,
  firstParameter,
  thirdParameter,
  seventhParameter,
  sixthParameter,
} from "../parameters"
import { RichParameter } from "../provisionerGenerated"

test("create workspace", async ({ page }) => {
  const template = await createTemplate(page, {
    apply: [
      {
        apply: {
          resources: [
            {
              name: "example",
            },
          ],
        },
      },
    ],
  })
  await createWorkspace(page, template)
})

test("create workspace with default immutable parameters", async ({ page }) => {
  const richParameters: RichParameter[] = [
    secondParameter,
    fourthParameter,
    fifthParameter,
  ]
  const template = await createTemplate(
    page,
    echoResponsesWithParameters(richParameters),
  )
  const workspaceName = await createWorkspace(page, template)
  await verifyParameters(page, workspaceName, richParameters, [
    { name: secondParameter.name, value: secondParameter.defaultValue },
    { name: fourthParameter.name, value: fourthParameter.defaultValue },
    { name: fifthParameter.name, value: fifthParameter.defaultValue },
  ])
})

test("create workspace with default mutable parameters", async ({ page }) => {
  const richParameters: RichParameter[] = [firstParameter, thirdParameter]
  const template = await createTemplate(
    page,
    echoResponsesWithParameters(richParameters),
  )
  const workspaceName = await createWorkspace(page, template)
  await verifyParameters(page, workspaceName, richParameters, [
    { name: firstParameter.name, value: firstParameter.defaultValue },
    { name: thirdParameter.name, value: thirdParameter.defaultValue },
  ])
})

test("create workspace with default and required parameters", async ({
  page,
}) => {
  const richParameters: RichParameter[] = [
    secondParameter,
    fourthParameter,
    sixthParameter,
    seventhParameter,
  ]
  const buildParameters = [
    { name: sixthParameter.name, value: "12345" },
    { name: seventhParameter.name, value: "abcdef" },
  ]
  const template = await createTemplate(
    page,
    echoResponsesWithParameters(richParameters),
  )
  const workspaceName = await createWorkspace(
    page,
    template,
    richParameters,
    buildParameters,
  )
  await verifyParameters(page, workspaceName, richParameters, [
    // user values:
    ...buildParameters,
    // default values:
    { name: secondParameter.name, value: secondParameter.defaultValue },
    { name: fourthParameter.name, value: fourthParameter.defaultValue },
  ])
})

test("create workspace and overwrite default parameters", async ({ page }) => {
  const richParameters: RichParameter[] = [secondParameter, fourthParameter]
  const buildParameters = [
    { name: secondParameter.name, value: "AAAAA" },
    { name: fourthParameter.name, value: "false" },
  ]
  const template = await createTemplate(
    page,
    echoResponsesWithParameters(richParameters),
  )
  const workspaceName = await createWorkspace(
    page,
    template,
    richParameters,
    buildParameters,
  )
  await verifyParameters(page, workspaceName, richParameters, buildParameters)
})
