import type { ChildProcess } from "node:child_process";
import { randomUUID } from "node:crypto";
import {
	expect,
	type Frame,
	type Page,
	type Response,
	test,
} from "@playwright/test";
import { API } from "#/api/api";
import type { Workspace } from "#/api/typesGenerated";
import type {
	NewPreviewTab,
	UserRightPanelTab,
} from "#/pages/AgentsPage/utils/rightPanelTabs";
import { setupApiCalls } from "../api";
import { users } from "../constants";
import {
	createTemplateVersionTar,
	login,
	randomName,
	startAgent,
	stopAgent,
} from "../helpers";
import { beforeCoderTest } from "../hooks";
import { AppSharingLevel } from "../provisionerGenerated";
import {
	escapeVectors,
	requestVectors,
	startCanaryApp,
	startHostilePreview,
} from "./fixtures";

const agentName = "dev";

// Containment checks are independent observations, so one failure still
// lets the rest report.
const softly = expect.configure({ soft: true });

test.beforeEach(async ({ page }) => {
	beforeCoderTest(page);
	await login(page);
});

test("a strict preview cannot reach the user's other apps or leave its wrapper", async ({
	page,
	context,
	baseURL,
}) => {
	if (!baseURL) {
		throw new Error("The previews config sets baseURL.");
	}
	const dashboard = new URL(baseURL);
	const canary = await startCanaryApp();
	const hostile = await startHostilePreview();
	let agent: ChildProcess | undefined;
	try {
		const token = randomUUID();
		const workspace = await createWorkspaceFromEcho(page, {
			graph: [
				{
					graph: {
						resources: [
							{
								agents: [
									{
										token,
										displayApps: { portForwardingHelper: true },
										apps: [
											{
												id: randomUUID(),
												slug: "canary",
												displayName: "Canary",
												url: `http://127.0.0.1:${canary.port}`,
												subdomain: true,
												sharingLevel: AppSharingLevel.OWNER,
												order: 0,
											},
										],
										order: 0,
									},
								],
							},
						],
					},
				},
			],
		});
		agent = await startAgent(page, token);

		const owner = users.owner.username;
		const previewSlug = `preview--${hostile.port}`;
		const previewOrigin = `${dashboard.protocol}//${previewSlug}--${agentName}--${workspace.name}--${owner}.${dashboard.host}`;
		const previewPolicy =
			"sandbox allow-scripts allow-same-origin allow-forms allow-modals allow-downloads; " +
			"default-src 'self' 'unsafe-inline' 'unsafe-eval' data: blob:; frame-src 'self'; " +
			`object-src 'none'; form-action 'self'; frame-ancestors 'self' ${dashboard.origin}`;
		// Read each policy as the response arrives, because Playwright cannot
		// read the headers of a response once its page has closed.
		const previewResponses: Promise<PolicyCheck>[] = [];
		context.on("response", (response) => {
			if (new URL(response.url()).origin === previewOrigin) {
				previewResponses.push(checkPolicy(response, previewPolicy));
			}
		});

		const canaryOrigin =
			await test.step("the canary counts a request sent with the user's credentials", async () => {
				const control = await context.newPage();
				const response = await control.goto(
					`${dashboard.protocol}//canary--${agentName}--${workspace.name}--${owner}.${dashboard.host}/control`,
				);
				await expect(
					control.getByRole("heading", { name: "Canary app" }),
				).toBeVisible();
				expect(canary.hits).toContainEqual({ method: "GET", path: "/control" });
				// The policy check below must be able to fail: an ordinary app's
				// page does not carry the preview policy.
				expect(
					response && (await checkPolicy(response, previewPolicy)).hasPolicy,
				).toBe(false);
				// The app sign-in may land on another spelling of the app's host,
				// and the user's credentials for the app live where it landed.
				const origin = new URL(control.url()).origin;
				await control.close();
				canary.hits.length = 0;
				return origin;
			});
		const attackPath = `/?${new URLSearchParams({ canary: canaryOrigin })}`;

		await test.step("opening the preview's address directly shows an error page", async () => {
			const direct = await context.newPage();
			const response = await direct.goto(`${previewOrigin}/`);
			expect(response?.status()).toBe(403);
			await direct.close();
			expect(hostile.paths()).toEqual([]);
		});

		const chatURL = await openChatWithPreview(page, workspace, {
			agentName,
			slug: previewSlug,
			urlPath: attackPath,
		});
		const preview =
			await test.step("the preview signs in inside its wrapper", async () => {
				await expect(
					previewContent(page).getByRole("heading", {
						name: "Hostile preview fixture",
					}),
				).toBeVisible({ timeout: 30_000 });
				const frame = await previewFrame(page);
				expect(new URL(frame.url()).origin).toBe(previewOrigin);
				return frame;
			});

		await test.step("no attack from the preview reaches the canary", async () => {
			const outcomes = await hostile.report;
			await test.info().attach("attack outcomes", {
				body: JSON.stringify(outcomes, null, "\t"),
				contentType: "application/json",
			});
			// Each request vector reached the preview's own server after its
			// canary attempt, so the browser really sends that kind of
			// request here and the canary attempt has settled. A vector that
			// never reaches its own server leaves the canary result
			// unproven, so it fails the test.
			await softly
				.poll(() =>
					requestVectors.filter(
						(vector) => !hostile.paths().includes(`/v/${vector}`),
					),
				)
				.toEqual([]);
			expect.soft(canary.hits).toEqual([]);
			// Escapes fail even toward the preview's own host, and coderd
			// refuses service worker scripts before the preview server sees
			// them.
			const escaped = [
				...escapeVectors,
				"service-worker",
				"early-hints",
				"link-header",
			].filter((vector) => hostile.paths().includes(`/v/${vector}`));
			expect.soft(escaped).toEqual([]);
		});

		await test.step("the wrapper keeps the preview and the dashboard in place", async () => {
			expect.soft(page.url()).toBe(chatURL);
			expect.soft(context.pages()).toHaveLength(1);
			expect
				.soft(new URL((await previewFrame(page)).url()).origin)
				.toBe(previewOrigin);
		});

		await test.step("the preview cannot navigate its own frame to another app", async () => {
			await navigateFrame(preview, `${previewOrigin}/v/selfnav`);
			await expect.poll(() => preview.url()).toBe(`${previewOrigin}/v/selfnav`);
			// The wrapper reports the navigation its frame-src blocked, so the
			// checks below run after the browser settled the attempt. Browsers
			// differ in where the blocked frame ends up.
			const wrapper = preview.parentFrame();
			if (!wrapper) {
				throw new Error("The preview frame has no wrapper.");
			}
			const blocked = wrapper.evaluate(
				() =>
					new Promise<string>((resolve) => {
						document.addEventListener(
							"securitypolicyviolation",
							(event) => resolve(event.effectiveDirective),
							{ once: true },
						);
					}),
			);
			await navigateFrame(preview, `${canaryOrigin}/v/selfnav`);
			expect(await blocked).toBe("frame-src");
			await test.info().attach("frame after blocked navigation", {
				body: preview.url(),
			});
			expect(new URL(preview.url()).origin).not.toBe(canaryOrigin);
			expect(canary.hits).toEqual([]);
			expect(page.url()).toBe(chatURL);
		});

		await test.step("every preview response carries the preview policy", async () => {
			const seen = await Promise.all(previewResponses);
			await test.info().attach("preview responses", {
				body: JSON.stringify(seen, null, "\t"),
				contentType: "application/json",
			});
			// The 200s come from the preview server, and the 303 and 403 are
			// coderd's sign-in redirect and direct-open page, so both sources
			// must carry the policy.
			expect(seen.map((response) => response.status)).toEqual(
				expect.arrayContaining([200, 303, 403]),
			);
			expect(seen.filter((response) => !response.hasPolicy)).toEqual([]);
		});
	} finally {
		if (agent) {
			await stopAgent(agent);
		}
		await hostile.close();
		await canary.close();
	}
});

/** previewContent locates the page inside the dashboard's preview wrapper. */
const previewContent = (page: Page) =>
	page
		.locator('iframe[title="Preview"]')
		.contentFrame()
		.locator('iframe[title="Preview"]')
		.contentFrame();

/** previewFrame returns the frame the dashboard's wrapper holds. */
const previewFrame = async (page: Page): Promise<Frame> => {
	const wrapper = await page
		.locator('iframe[title="Preview"]')
		.elementHandle()
		.then((element) => element?.contentFrame());
	const preview = wrapper?.childFrames()[0];
	if (!preview) {
		throw new Error("The dashboard has no preview wrapper with a frame.");
	}
	return preview;
};

/** navigateFrame makes the page in frame navigate itself to url. */
const navigateFrame = (frame: Frame, url: string) =>
	frame.evaluate((target) => {
		setTimeout(() => location.assign(target));
	}, url);

type PolicyCheck = { url: string; status: number; hasPolicy: boolean };

const checkPolicy = async (
	response: Response,
	policy: string,
): Promise<PolicyCheck> => ({
	url: response.url(),
	status: response.status(),
	hasPolicy: (await response.headerValues("content-security-policy")).includes(
		policy,
	),
});

/**
 * openChatWithPreview opens an idle chat on workspace with the preview
 * restored in its right panel, shows the preview, and returns the chat's URL.
 * The chat never runs, so its model's provider needs no server.
 */
const openChatWithPreview = async (
	page: Page,
	workspace: Workspace,
	preview: Pick<NewPreviewTab, "agentName" | "slug" | "urlPath">,
): Promise<string> => {
	const provider = await API.experimental.createAIProvider({
		type: "openai",
		name: randomName("previews"),
		enabled: true,
		base_url: "http://127.0.0.1:1",
		api_keys: ["unused"],
	});
	await API.experimental.createChatModel(workspace.organization_id, {
		ai_provider_id: provider.id,
		model: "unused",
		is_default: true,
		context_limit: 100_000,
	});
	const chat = await API.experimental.createChat({
		organization_id: workspace.organization_id,
		workspace_id: workspace.id,
		content: [],
	});
	const tabs: UserRightPanelTab[] = [
		{
			id: "preview",
			kind: "preview",
			label: "Preview",
			workspaceId: workspace.id,
			...preview,
		},
	];
	await page.evaluate(
		([chatId, tabsJSON]) => {
			localStorage.setItem("agents.right-panel-open", "true");
			localStorage.setItem(`agents.right-panel-tabs.${chatId}`, tabsJSON);
		},
		[chat.id, JSON.stringify(tabs)],
	);
	await page.goto(`/agents/${chat.id}`);
	await page.getByRole("tab", { name: "Preview" }).click();
	return page.url();
};

/**
 * createWorkspaceFromEcho creates a template from echo provisioner responses
 * and a workspace from it through the API, opens the workspace page, and
 * returns the workspace.
 * The template upload form fails in WebKit, and the form is not under test.
 */
const createWorkspaceFromEcho = async (
	page: Page,
	responses: Parameters<typeof createTemplateVersionTar>[0],
): Promise<Workspace> => {
	await setupApiCalls(page);
	const me = await API.getAuthenticatedUser();
	const organization = me.organization_ids[0];
	const tar = await createTemplateVersionTar(responses);
	const upload = await API.uploadFile(
		new File([new Uint8Array(tar)], "template.tar", {
			type: "application/x-tar",
		}),
	);
	const version = await API.createTemplateVersion(organization, {
		storage_method: "file",
		file_id: upload.hash,
		provisioner: "echo",
		tags: {},
	});
	await expect
		.poll(async () => (await API.getTemplateVersion(version.id)).job.status, {
			timeout: 30_000,
		})
		.toBe("succeeded");
	const template = await API.createTemplate(organization, {
		name: randomName("previews"),
		template_version_id: version.id,
		allow_user_cancel_workspace_jobs: null,
		disable_everyone_group_access: false,
		require_active_version: false,
		max_port_share_level: null,
		cors_behavior: null,
	});
	const workspace = await API.createWorkspace(me.id, {
		template_id: template.id,
		name: randomName(),
	});
	await API.waitForBuild(workspace.latest_build);
	// startAgent watches the workspace page for the agent to connect.
	await page.goto(`/@${me.username}/${workspace.name}`);
	return workspace;
};
