import { expect, test } from "@playwright/test";
import { username } from "../../constants";
import {
	createTemplate,
	createWorkspace,
	echoResponsesWithParameters,
} from "../../helpers";
import { emptyParameter } from "../../parameters";
import type { RichParameter } from "../../provisionerGenerated";

test("create workspace in auto mode", async ({ page }) => {
	const richParameters: RichParameter[] = [
		{ ...emptyParameter, name: "repo", type: "string" },
	];
	const template = await createTemplate(
		page,
		echoResponsesWithParameters(richParameters),
	);
	const name = "test-workspace";
	await page.goto(
		`/templates/${template}/workspace?mode=auto&param.repo=example&name=${name}`,
		{
			waitUntil: "domcontentloaded",
		},
	);
	await expect(page).toHaveTitle(`${username}/${name} - Coder`);
});

test("use an existing workspace that matches the `match` parameter instead of creating a new one", async ({
	page,
}) => {
	const richParameters: RichParameter[] = [
		{ ...emptyParameter, name: "repo", type: "string" },
	];
	const template = await createTemplate(
		page,
		echoResponsesWithParameters(richParameters),
	);
	const prevWorkspace = await createWorkspace(page, template);
	await page.goto(
		`/templates/${template}/workspace?mode=auto&param.repo=example&name=new-name&match=name:${prevWorkspace}`,
		{
			waitUntil: "domcontentloaded",
		},
	);
	await expect(page).toHaveTitle(`${username}/${prevWorkspace} - Coder`);
});

test("show error if `match` parameter is invalid", async ({ page }) => {
	const richParameters: RichParameter[] = [
		{ ...emptyParameter, name: "repo", type: "string" },
	];
	const template = await createTemplate(
		page,
		echoResponsesWithParameters(richParameters),
	);
	const prevWorkspace = await createWorkspace(page, template);
	await page.goto(
		`/templates/${template}/workspace?mode=auto&param.repo=example&name=new-name&match=not-valid-query:${prevWorkspace}`,
		{
			waitUntil: "domcontentloaded",
		},
	);
	await expect(page.getByText("Invalid match value")).toBeVisible();
});
