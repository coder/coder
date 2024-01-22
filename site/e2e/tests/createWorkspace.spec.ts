import { test, expect } from "@playwright/test";
import {
  createTemplate,
  createWorkspace,
  echoResponsesWithParameters,
  verifyParameters,
} from "../helpers";

import {
  secondParameter,
  fourthParameter,
  fifthParameter,
  firstParameter,
  thirdParameter,
  seventhParameter,
  sixthParameter,
  randParamName,
} from "../parameters";
import { RichParameter } from "../provisionerGenerated";
import { beforeCoderTest } from "../hooks";

test.beforeEach(async ({ page }) => await beforeCoderTest(page));

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
  });
  await createWorkspace(page, template);
});

test("create workspace with default immutable parameters", async ({ page }) => {
  const richParameters: RichParameter[] = [
    secondParameter,
    fourthParameter,
    fifthParameter,
  ];
  const template = await createTemplate(
    page,
    echoResponsesWithParameters(richParameters),
  );
  const workspaceName = await createWorkspace(page, template);
  await verifyParameters(page, workspaceName, richParameters, [
    { name: secondParameter.name, value: secondParameter.defaultValue },
    { name: fourthParameter.name, value: fourthParameter.defaultValue },
    { name: fifthParameter.name, value: fifthParameter.defaultValue },
  ]);
});

test("create workspace with default mutable parameters", async ({ page }) => {
  const richParameters: RichParameter[] = [firstParameter, thirdParameter];
  const template = await createTemplate(
    page,
    echoResponsesWithParameters(richParameters),
  );
  const workspaceName = await createWorkspace(page, template);
  await verifyParameters(page, workspaceName, richParameters, [
    { name: firstParameter.name, value: firstParameter.defaultValue },
    { name: thirdParameter.name, value: thirdParameter.defaultValue },
  ]);
});

test("create workspace with default and required parameters", async ({
  page,
}) => {
  const richParameters: RichParameter[] = [
    secondParameter,
    fourthParameter,
    sixthParameter,
    seventhParameter,
  ];
  const buildParameters = [
    { name: sixthParameter.name, value: "12345" },
    { name: seventhParameter.name, value: "abcdef" },
  ];
  const template = await createTemplate(
    page,
    echoResponsesWithParameters(richParameters),
  );
  const workspaceName = await createWorkspace(
    page,
    template,
    richParameters,
    buildParameters,
  );
  await verifyParameters(page, workspaceName, richParameters, [
    // user values:
    ...buildParameters,
    // default values:
    { name: secondParameter.name, value: secondParameter.defaultValue },
    { name: fourthParameter.name, value: fourthParameter.defaultValue },
  ]);
});

test("create workspace and overwrite default parameters", async ({ page }) => {
  // We use randParamName to prevent the new values from corrupting user_history
  // and thus affecting other tests.
  const richParameters: RichParameter[] = [
    randParamName(secondParameter),
    randParamName(fourthParameter),
  ];

  const buildParameters = [
    { name: richParameters[0].name, value: "AAAAA" },
    { name: richParameters[1].name, value: "false" },
  ];
  const template = await createTemplate(
    page,
    echoResponsesWithParameters(richParameters),
  );

  const workspaceName = await createWorkspace(
    page,
    template,
    richParameters,
    buildParameters,
  );
  await verifyParameters(page, workspaceName, richParameters, buildParameters);
});

test("create workspace with disable_param search params", async ({ page }) => {
  const richParameters: RichParameter[] = [
    firstParameter, // mutable
    secondParameter, //immutable
  ];

  const templateName = await createTemplate(
    page,
    echoResponsesWithParameters(richParameters),
  );

  await page.goto(
    `/templates/${templateName}/workspace?disable_params=first_parameter,second_parameter`,
    {
      waitUntil: "domcontentloaded",
    },
  );

  await expect(page.getByLabel(/First parameter/i)).toBeDisabled();
  await expect(page.getByLabel(/Second parameter/i)).toBeDisabled();
});
