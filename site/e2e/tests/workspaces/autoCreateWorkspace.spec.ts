import { expect, test } from "@playwright/test";
import { users } from "../../constants";
import {
	createTemplate,
	createWorkspace,
	echoResponsesWithParameters,
} from "../../helpers";
import { login } from "../../helpers";
import { beforeCoderTest } from "../../hooks";
import { emptyParameter } from "../../parameters";
import type { RichParameter } from "../../provisionerGenerated";

test.describe.configure({ mode: "parallel" });

let template!: string;

test.beforeAll(async ({ browser }) => {
	const page = await (await browser.newContext()).newPage();
	await login(page, users.templateAdmin);

	const richParameters: RichParameter[] = [
		{ ...emptyParameter, name: "repo", type: "string" },
	];
	template = await createTemplate(
		page,
		echoResponsesWithParameters(richParameters),
	);
});

test.beforeEach(async ({ page }) => {
	beforeCoderTest(page);
	await login(page, users.member);
});

test("create workspace in auto mode", async ({ page }) => {
	const name = "test-workspace";
	await page.goto(
		`/templates/${template}/workspace?mode=auto&param.repo=example&name=${name}`,
		{
			waitUntil: "domcontentloaded",
		},
	);
	await expect(page).toHaveTitle(`${users.member.username}/${name} - Coder`);
});

test("use an existing workspace that matches the `match` parameter instead of creating a new one", async ({
	page,
}) => {
	const prevWorkspace = await createWorkspace(page, template);
	await page.goto(
		`/templates/${template}/workspace?mode=auto&param.repo=example&name=new-name&match=name:${prevWorkspace}`,
		{
			waitUntil: "domcontentloaded",
		},
	);
	await expect(page).toHaveTitle(
		`${users.member.username}/${prevWorkspace} - Coder`,
	);
});

test("show error if `match` parameter is invalid", async ({ page }) => {
	const prevWorkspace = await createWorkspace(page, template);
	await page.goto(
		`/templates/${template}/workspace?mode=auto&param.repo=example&name=new-name&match=not-valid-query:${prevWorkspace}`,
		{
			waitUntil: "domcontentloaded",
		},
	);
	await expect(page.getByText("Invalid match value")).toBeVisible();
});
