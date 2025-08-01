import { test } from "@playwright/test";
import { users } from "../../constants";
import {
	createTemplate,
	createWorkspace,
	disableDynamicParameters,
	echoResponsesWithParameters,
	updateTemplate,
	updateWorkspace,
	updateWorkspaceParameters,
	verifyParameters,
} from "../../helpers";
import { login } from "../../helpers";
import { beforeCoderTest } from "../../hooks";
import {
	fifthParameter,
	firstParameter,
	secondBuildOption,
	secondParameter,
	sixthParameter,
} from "../../parameters";
import type { RichParameter } from "../../provisionerGenerated";

test.beforeEach(async ({ page }) => {
	beforeCoderTest(page);
});

test("update workspace, new optional, immutable parameter added", async ({
	page,
}) => {
	await login(page, users.templateAdmin);
	const richParameters: RichParameter[] = [firstParameter, secondParameter];
	const template = await createTemplate(
		page,
		echoResponsesWithParameters(richParameters),
	);

	// Disable dynamic parameters to use classic parameter flow for this test
	await disableDynamicParameters(page, template);

	await login(page, users.member);
	const workspaceName = await createWorkspace(page, template);

	// Verify that parameter values are default.
	await verifyParameters(page, workspaceName, richParameters, [
		{ name: firstParameter.name, value: firstParameter.defaultValue },
		{ name: secondParameter.name, value: secondParameter.defaultValue },
	]);

	// Push updated template.
	await login(page, users.templateAdmin);
	const updatedRichParameters = [...richParameters, fifthParameter];
	await updateTemplate(
		page,
		"coder",
		template,
		echoResponsesWithParameters(updatedRichParameters),
	);

	// Now, update the workspace, and select the value for immutable parameter.
	await login(page, users.member);
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
	await login(page, users.templateAdmin);
	const richParameters: RichParameter[] = [firstParameter, secondParameter];
	const template = await createTemplate(
		page,
		echoResponsesWithParameters(richParameters),
	);

	// Disable dynamic parameters to use classic parameter flow for this test
	await disableDynamicParameters(page, template);

	await login(page, users.member);
	const workspaceName = await createWorkspace(page, template);

	// Verify that parameter values are default.
	await verifyParameters(page, workspaceName, richParameters, [
		{ name: firstParameter.name, value: firstParameter.defaultValue },
		{ name: secondParameter.name, value: secondParameter.defaultValue },
	]);

	// Push updated template.
	await login(page, users.templateAdmin);
	const updatedRichParameters = [...richParameters, sixthParameter];
	await updateTemplate(
		page,
		"coder",
		template,
		echoResponsesWithParameters(updatedRichParameters),
	);

	// Now, update the workspace, and provide the parameter value.
	await login(page, users.member);
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
	await login(page, users.templateAdmin);
	const richParameters: RichParameter[] = [firstParameter, secondBuildOption];
	const template = await createTemplate(
		page,
		echoResponsesWithParameters(richParameters),
	);

	// Disable dynamic parameters to use classic parameter flow for this test
	await disableDynamicParameters(page, template);

	await login(page, users.member);
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
