import { test } from "@playwright/test";
import { users } from "../../constants";
import {
	buildWorkspaceWithParameters,
	createTemplate,
	createWorkspace,
	echoResponsesWithParameters,
	verifyParameters,
} from "../../helpers";
import { login } from "../../helpers";
import { beforeCoderTest } from "../../hooks";
import { firstBuildOption, secondBuildOption } from "../../parameters";
import type { RichParameter } from "../../provisionerGenerated";

test.beforeEach(async ({ page }) => {
	beforeCoderTest(page);
});

test("restart workspace with ephemeral parameters", async ({ page }) => {
	await login(page, users.templateAdmin);
	const richParameters: RichParameter[] = [firstBuildOption, secondBuildOption];
	const template = await createTemplate(
		page,
		echoResponsesWithParameters(richParameters),
	);

	await login(page, users.member);
	const workspaceName = await createWorkspace(page, template);

	// Verify that build options are default (not selected).
	await verifyParameters(page, workspaceName, richParameters, [
		{ name: richParameters[0].name, value: firstBuildOption.defaultValue },
		{ name: richParameters[1].name, value: secondBuildOption.defaultValue },
	]);

	// Now, restart the workspace with ephemeral parameters selected.
	const buildParameters = [
		{ name: richParameters[0].name, value: "AAAAA" },
		{ name: richParameters[1].name, value: "true" },
	];
	await buildWorkspaceWithParameters(
		page,
		workspaceName,
		richParameters,
		buildParameters,
		true,
	);

	// Verify that build options are default (not selected).
	await verifyParameters(page, workspaceName, richParameters, [
		{ name: richParameters[0].name, value: firstBuildOption.defaultValue },
		{ name: richParameters[1].name, value: secondBuildOption.defaultValue },
	]);
});
