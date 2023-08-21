import { test } from "@playwright/test"
import { createTemplate, createWorkspace, verifyParameters } from "../helpers"

import { secondParameter, fourthParameter, sixthParameter } from "../parameters"
import { RichParameter } from "../provisionerGenerated"

test("create workspace", async ({ page }) => {
  const template = await createTemplate(page, {
    apply: [
      {
        complete: {
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
  const richParameters: RichParameter[] = [secondParameter, fourthParameter, sixthParameter]
  const template = await createTemplate(page, {
    plan: [
      {
        complete: {
          parameters: richParameters,
        },
      },
    ],
    apply: [
      {
        complete: {
          resources: [
            {
              name: "example",
            },
          ],
        },
      },
    ],
  })
  const workspaceName = await createWorkspace(page, template)
  await verifyParameters(page, workspaceName, richParameters, [
    { name: secondParameter.name, value: secondParameter.defaultValue },
    { name: fourthParameter.name, value: fourthParameter.defaultValue },
    { name: sixthParameter.name, value: sixthParameter.defaultValue },
  ])
})
