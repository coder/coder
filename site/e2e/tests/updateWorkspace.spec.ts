import { test } from "@playwright/test";

import {
  createTemplate,
  createWorkspace,
  echoResponsesWithParameters,
  updateTemplate,
  updateWorkspace,
  updateWorkspaceParameters,
  verifyParameters,
} from "../helpers";

import {
  fifthParameter,
  firstParameter,
  secondParameter,
  sixthParameter,
  secondBuildOption,
} from "../parameters";
import { RichParameter } from "../provisionerGenerated";
import { beforeCoderTest } from "../hooks";

test.beforeEach(async ({ page }) => await beforeCoderTest(page));

test("update workspace, new optional, immutable parameter added", async ({
  page,
}) => {
  const richParameters: RichParameter[] = [firstParameter, secondParameter];
  const template = await createTemplate(
    page,
    echoResponsesWithParameters(richParameters),
  );

  const workspaceName = await createWorkspace(page, template);

  // Verify that parameter values are default.
  await verifyParameters(page, workspaceName, richParameters, [
    { name: firstParameter.name, value: firstParameter.defaultValue },
    { name: secondParameter.name, value: secondParameter.defaultValue },
  ]);

  // Push updated template.
  const updatedRichParameters = [...richParameters, fifthParameter];
  await updateTemplate(
    page,
    template,
    echoResponsesWithParameters(updatedRichParameters),
  );

  // Now, update the workspace, and select the value for immutable parameter.
  await updateWorkspace(page, workspaceName, updatedRichParameters, [
    { name: fifthParameter.name, value: fifthParameter.options[0].value },
  ]);

  // Verify parameter values.
  await verifyParameters(page, workspaceName, updatedRichParameters, [
    { name: firstParameter.name, value: firstParameter.defaultValue },
    { name: secondParameter.name, value: secondParameter.defaultValue },
    { name: fifthParameter.name, value: fifthParameter.options[0].value },
  ]);
});

test("update workspace, new required, mutable parameter added", async ({
  page,
}) => {
  const richParameters: RichParameter[] = [firstParameter, secondParameter];
  const template = await createTemplate(
    page,
    echoResponsesWithParameters(richParameters),
  );

  const workspaceName = await createWorkspace(page, template);

  // Verify that parameter values are default.
  await verifyParameters(page, workspaceName, richParameters, [
    { name: firstParameter.name, value: firstParameter.defaultValue },
    { name: secondParameter.name, value: secondParameter.defaultValue },
  ]);

  // Push updated template.
  const updatedRichParameters = [...richParameters, sixthParameter];
  await updateTemplate(
    page,
    template,
    echoResponsesWithParameters(updatedRichParameters),
  );

  // Now, update the workspace, and provide the parameter value.
  const buildParameters = [{ name: sixthParameter.name, value: "99" }];
  await updateWorkspace(
    page,
    workspaceName,
    updatedRichParameters,
    buildParameters,
  );

  // Verify parameter values.
  await verifyParameters(page, workspaceName, updatedRichParameters, [
    { name: firstParameter.name, value: firstParameter.defaultValue },
    { name: secondParameter.name, value: secondParameter.defaultValue },
    ...buildParameters,
  ]);
});

test("update workspace with ephemeral parameter enabled", async ({ page }) => {
  const richParameters: RichParameter[] = [firstParameter, secondBuildOption];
  const template = await createTemplate(
    page,
    echoResponsesWithParameters(richParameters),
  );

  const workspaceName = await createWorkspace(page, template);

  // Verify that parameter values are default.
  await verifyParameters(page, workspaceName, richParameters, [
    { name: firstParameter.name, value: firstParameter.defaultValue },
    { name: secondBuildOption.name, value: secondBuildOption.defaultValue },
  ]);

  // Now, update the workspace, and select the value for ephemeral parameter.
  const buildParameters = [{ name: secondBuildOption.name, value: "true" }];
  await updateWorkspaceParameters(
    page,
    workspaceName,
    richParameters,
    buildParameters,
  );

  // Verify that parameter values are default.
  await verifyParameters(page, workspaceName, richParameters, [
    { name: firstParameter.name, value: firstParameter.defaultValue },
    { name: secondBuildOption.name, value: secondBuildOption.defaultValue },
  ]);
});
