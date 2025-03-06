import { type ChildProcess, exec, spawn } from "node:child_process";
import { randomUUID } from "node:crypto";
import net from "node:net";
import path from "node:path";
import { Duplex } from "node:stream";
import { type BrowserContext, type Page, expect, test } from "@playwright/test";
import { API } from "api/api";
import type {
	UpdateTemplateMeta,
	WorkspaceBuildParameter,
} from "api/typesGenerated";
import express from "express";
import capitalize from "lodash/capitalize";
import * as ssh from "ssh2";
import { TarWriter } from "utils/tar";
import {
	agentPProfPort,
	coderBinary,
	coderPort,
	defaultOrganizationName,
	defaultPassword,
	license,
	premiumTestsRequired,
	prometheusPort,
	requireTerraformTests,
	users,
} from "./constants";
import { expectUrl } from "./expectUrl";
import {
	Agent,
	type App,
	AppSharingLevel,
	type ApplyComplete,
	type ExternalAuthProviderResource,
	type ParseComplete,
	type PlanComplete,
	type Resource,
	Response,
	type RichParameter,
} from "./provisionerGenerated";

/**
 * requiresLicense will skip the test if we're not running with a license added
 */
export function requiresLicense() {
	if (premiumTestsRequired) {
		return;
	}

	test.skip(!license);
}

export function requiresUnlicensed() {
	test.skip(license.length > 0);
}

/**
 * requireTerraformProvisioner by default is enabled.
 */
export function requireTerraformProvisioner() {
	test.skip(!requireTerraformTests);
}

export type LoginOptions = {
	username: string;
	email: string;
	password: string;
};

export async function login(page: Page, options: LoginOptions = users.admin) {
	const ctx = page.context();
	// biome-ignore lint/suspicious/noExplicitAny: reset the current user
	(ctx as any)[Symbol.for("currentUser")] = undefined;
	await ctx.clearCookies();
	await page.goto("/login");
	await page.getByLabel("Email").fill(options.email);
	await page.getByLabel("Password").fill(options.password);
	await page.getByRole("button", { name: "Sign In" }).click();
	await expectUrl(page).toHavePathName("/workspaces");
	// biome-ignore lint/suspicious/noExplicitAny: update once logged in
	(ctx as any)[Symbol.for("currentUser")] = options;
}

export function currentUser(page: Page): LoginOptions {
	const ctx = page.context();
	// biome-ignore lint/suspicious/noExplicitAny: get the current user
	const user = (ctx as any)[Symbol.for("currentUser")];

	if (!user) {
		throw new Error("page context does not have a user. did you call `login`?");
	}

	return user;
}

type CreateWorkspaceOptions = {
	richParameters?: RichParameter[];
	buildParameters?: WorkspaceBuildParameter[];
	useExternalAuth?: boolean;
};

/**
 * createWorkspace creates a workspace for a template. It does not wait for it
 * to be running, but it does navigate to the page.
 */
export const createWorkspace = async (
	page: Page,
	template: string | { organization: string; name: string },
	options: CreateWorkspaceOptions = {},
): Promise<string> => {
	const {
		richParameters = [],
		buildParameters = [],
		useExternalAuth,
	} = options;

	const templatePath =
		typeof template === "string"
			? template
			: `${template.organization}/${template.name}`;

	await page.goto(`/templates/${templatePath}/workspace`, {
		waitUntil: "domcontentloaded",
	});
	await expectUrl(page).toHavePathName(`/templates/${templatePath}/workspace`);

	const name = randomName();
	await page.getByLabel("name").fill(name);

	await fillParameters(page, richParameters, buildParameters);

	if (useExternalAuth) {
		// Create a new context for the popup which will be created when clicking the button
		const popupPromise = page.waitForEvent("popup");

		// Find the "Login with <Provider>" button
		const externalAuthLoginButton = page
			.getByRole("button")
			.getByText("Login with GitHub");
		await expect(externalAuthLoginButton).toBeVisible();

		// Click it
		await externalAuthLoginButton.click();

		// Wait for authentication to occur
		const popup = await popupPromise;
		await popup.waitForSelector("text=You are now authenticated.");
	}

	await page.getByRole("button", { name: /create workspace/i }).click();

	const user = currentUser(page);
	await expectUrl(page).toHavePathName(`/@${user.username}/${name}`);

	await page.waitForSelector("[data-testid='build-status'] >> text=Running", {
		state: "visible",
	});
	return name;
};

export const verifyParameters = async (
	page: Page,
	workspaceName: string,
	richParameters: RichParameter[],
	expectedBuildParameters: WorkspaceBuildParameter[],
) => {
	const user = currentUser(page);
	await page.goto(`/@${user.username}/${workspaceName}/settings/parameters`, {
		waitUntil: "domcontentloaded",
	});

	for (const buildParameter of expectedBuildParameters) {
		const richParameter = richParameters.find(
			(richParam) => richParam.name === buildParameter.name,
		);
		if (!richParameter) {
			throw new Error(
				"build parameter is expected to be present in rich parameter schema",
			);
		}

		const parameterLabel = await page.waitForSelector(
			`[data-testid='parameter-field-${richParameter.name}']`,
			{ state: "visible" },
		);

		const muiDisabled = richParameter.mutable ? "" : ".Mui-disabled";

		if (richParameter.type === "bool") {
			const parameterField = await parameterLabel.waitForSelector(
				`[data-testid='parameter-field-bool'] .MuiRadio-root.Mui-checked${muiDisabled} input`,
			);
			const value = await parameterField.inputValue();
			expect(value).toEqual(buildParameter.value);
		} else if (richParameter.options.length > 0) {
			const parameterField = await parameterLabel.waitForSelector(
				`[data-testid='parameter-field-options'] .MuiRadio-root.Mui-checked${muiDisabled} input`,
			);
			const value = await parameterField.inputValue();
			expect(value).toEqual(buildParameter.value);
		} else if (richParameter.type === "list(string)") {
			throw new Error("not implemented yet"); // FIXME
		} else {
			// text or number
			const parameterField = await parameterLabel.waitForSelector(
				`[data-testid='parameter-field-text'] input${muiDisabled}`,
			);
			const value = await parameterField.inputValue();
			expect(value).toEqual(buildParameter.value);
		}
	}
};

/**
 * StarterTemplates are ids of starter templates that can be used in place of
 * the responses payload. These starter templates will require real provisioners.
 */
export enum StarterTemplates {
	STARTER_DOCKER = "docker",
}

function isStarterTemplate(
	input: EchoProvisionerResponses | StarterTemplates | undefined,
): input is StarterTemplates {
	if (!input) {
		return false;
	}
	return typeof input === "string";
}

/**
 * createTemplate navigates to the /templates/new page and uploads a template
 * with the resources provided in the responses argument.
 */
export const createTemplate = async (
	page: Page,
	responses?: EchoProvisionerResponses | StarterTemplates,
	orgName = defaultOrganizationName,
): Promise<string> => {
	let path = "/templates/new";
	if (isStarterTemplate(responses)) {
		path += `?exampleId=${responses}`;
	} else {
		// The form page will read this value and use it as the default type.
		path += "?provisioner_type=echo";
	}

	await page.goto(path, { waitUntil: "domcontentloaded" });
	await expectUrl(page).toHavePathName("/templates/new");

	if (!isStarterTemplate(responses)) {
		await page.getByTestId("file-upload").setInputFiles({
			buffer: await createTemplateVersionTar(responses),
			mimeType: "application/x-tar",
			name: "template.tar",
		});
	}

	// If the organization picker is present on the page, select the default
	// organization.
	const orgPicker = page.getByLabel("Belongs to *");
	const organizationsEnabled = await orgPicker.isVisible();
	if (organizationsEnabled) {
		if (orgName !== defaultOrganizationName) {
			throw new Error(
				`No provisioners registered for ${orgName}, creating this template will fail`,
			);
		}

		// picker is disabled if only one org is available
		const pickerIsDisabled = await orgPicker.isDisabled();

		if (!pickerIsDisabled) {
			await orgPicker.click();
			await page.getByText(orgName, { exact: true }).click();
		}
	}

	const name = randomName();
	await page.getByLabel("Name *").fill(name);
	await page.getByRole("button", { name: /save/i }).click();
	await expectUrl(page).toHavePathName(
		organizationsEnabled
			? `/templates/${orgName}/${name}/files`
			: `/templates/${name}/files`,
		{
			timeout: 30000,
		},
	);
	return name;
};

/**
 * createGroup navigates to the /groups/create page and creates a group with a
 * random name.
 */
export const createGroup = async (
	page: Page,
	organization?: string,
): Promise<string> => {
	const prefix = organization
		? `/organizations/${organization}`
		: "/deployment";
	await page.goto(`${prefix}/groups/create`, {
		waitUntil: "domcontentloaded",
	});
	await expectUrl(page).toHavePathName(`${prefix}/groups/create`);

	const name = randomName();
	await page.getByLabel("Name", { exact: true }).fill(name);
	await page.getByRole("button", { name: /save/i }).click();
	await expectUrl(page).toHavePathName(`${prefix}/groups/${name}`);
	return name;
};

/**
 * sshIntoWorkspace spawns a Coder SSH process and a client connected to it.
 */
export const sshIntoWorkspace = async (
	page: Page,
	workspace: string,
	binaryPath = coderBinary,
	binaryArgs: string[] = [],
): Promise<ssh.Client> => {
	const sessionToken = await findSessionToken(page);
	return new Promise<ssh.Client>((resolve, reject) => {
		const cp = spawn(binaryPath, [...binaryArgs, "ssh", "--stdio", workspace], {
			env: {
				...process.env,
				CODER_SESSION_TOKEN: sessionToken,
				CODER_URL: `http://localhost:${coderPort}`,
			},
		});
		cp.on("error", (err) => reject(err));
		const proxyStream = new Duplex({
			read: (size) => {
				return cp.stdout.read(Math.min(size, cp.stdout.readableLength));
			},
			write: cp.stdin.write.bind(cp.stdin),
		});
		cp.stderr.on("data", (data) => console.info(data.toString()));
		cp.stdout.on("readable", (...args) => {
			proxyStream.emit("readable", ...args);
			if (cp.stdout.readableLength > 0) {
				proxyStream.emit("data", cp.stdout.read());
			}
		});
		const client = new ssh.Client();
		client.connect({
			sock: proxyStream,
			username: "coder",
		});
		client.on("error", (err) => reject(err));
		client.on("ready", () => {
			resolve(client);
		});
	});
};

export const stopWorkspace = async (page: Page, workspaceName: string) => {
	const user = currentUser(page);
	await page.goto(`/@${user.username}/${workspaceName}`, {
		waitUntil: "domcontentloaded",
	});

	await page.getByTestId("workspace-stop-button").click();

	await page.waitForSelector("*[data-testid='build-status'] >> text=Stopped", {
		state: "visible",
	});
};

export const buildWorkspaceWithParameters = async (
	page: Page,
	workspaceName: string,
	richParameters: RichParameter[] = [],
	buildParameters: WorkspaceBuildParameter[] = [],
	confirm = false,
) => {
	const user = currentUser(page);
	await page.goto(`/@${user.username}/${workspaceName}`, {
		waitUntil: "domcontentloaded",
	});

	await page.getByTestId("build-parameters-button").click();

	await fillParameters(page, richParameters, buildParameters);
	await page.getByTestId("build-parameters-submit").click();
	if (confirm) {
		await page.getByTestId("confirm-button").click();
	}

	await page.waitForSelector("*[data-testid='build-status'] >> text=Running", {
		state: "visible",
	});
};

/**
 * startAgent runs the coder agent with the provided token. It waits for the
 * agent to be ready before returning.
 */
export const startAgent = async (
	page: Page,
	token: string,
): Promise<ChildProcess> => {
	return startAgentWithCommand(page, token, coderBinary);
};

/**
 * downloadCoderVersion downloads the version provided into a temporary dir and
 * caches it so subsequent calls are fast.
 */
export const downloadCoderVersion = async (
	version: string,
): Promise<string> => {
	if (version.startsWith("v")) {
		version = version.slice(1);
	}

	const binaryName = `coder-e2e-${version}`;
	const tempDir = "/tmp/coder-e2e-cache";
	// The install script adds `./bin` automatically to the path :shrug:
	const binaryPath = path.join(tempDir, "bin", binaryName);

	const exists = await new Promise<boolean>((resolve) => {
		const cp = spawn(binaryPath, ["version"]);
		cp.on("close", (code) => {
			resolve(code === 0);
		});
		cp.on("error", () => resolve(false));
	});
	if (exists) {
		return binaryPath;
	}

	// Run our official install script to install the binary
	await new Promise<void>((resolve, reject) => {
		const cp = spawn(
			path.join(__dirname, "../../install.sh"),
			[
				"--version",
				version,
				"--method",
				"standalone",
				"--prefix",
				tempDir,
				"--binary-name",
				binaryName,
			],
			{
				env: {
					...process.env,
					XDG_CACHE_HOME: "/tmp/coder-e2e-cache",
					TRACE: "1", // tells install.sh to `set -x`, helpful if something goes wrong
				},
			},
		);
		cp.stderr.on("data", (data) => console.error(data.toString()));
		cp.stdout.on("data", (data) => console.info(data.toString()));
		cp.on("close", (code) => {
			if (code === 0) {
				resolve();
			} else {
				reject(new Error(`install.sh failed with code ${code}`));
			}
		});
	});
	return binaryPath;
};

export const startAgentWithCommand = async (
	page: Page,
	token: string,
	command: string,
	...args: string[]
): Promise<ChildProcess> => {
	const cp = spawn(command, [...args, "agent", "--no-reap"], {
		env: {
			...process.env,
			CODER_AGENT_URL: `http://localhost:${coderPort}`,
			CODER_AGENT_TOKEN: token,
			CODER_AGENT_PPROF_ADDRESS: `127.0.0.1:${agentPProfPort}`,
			CODER_AGENT_PROMETHEUS_ADDRESS: `127.0.0.1:${prometheusPort}`,
		},
	});
	cp.stdout.on("data", (data: Buffer) => {
		console.info(`[agent][stdout] ${data.toString().replace(/\n$/g, "")}`);
	});
	cp.stderr.on("data", (data: Buffer) => {
		console.info(`[agent][stderr] ${data.toString().replace(/\n$/g, "")}`);
	});

	await page
		.getByTestId("agent-status-ready")
		.waitFor({ state: "visible", timeout: 15_000 });
	return cp;
};

export const stopAgent = async (cp: ChildProcess) => {
	// The command `kill` is used to terminate an agent started as a standalone binary.
	exec(`kill ${cp.pid}`, (error) => {
		if (error) {
			throw new Error(`exec error: ${JSON.stringify(error)}`);
		}
	});
	await waitUntilUrlIsNotResponding(`http://localhost:${prometheusPort}`);
};

export const waitUntilUrlIsNotResponding = async (url: string) => {
	const maxRetries = 30;
	const retryIntervalMs = 1000;
	let retries = 0;

	const axiosInstance = API.getAxiosInstance();
	while (retries < maxRetries) {
		try {
			await axiosInstance.get(url);
		} catch {
			return;
		}

		retries++;
		await new Promise((resolve) => setTimeout(resolve, retryIntervalMs));
	}
	throw new Error(
		`URL ${url} is still responding after ${maxRetries * retryIntervalMs}ms`,
	);
};

// Allows users to more easily define properties they want for agents and resources!
type RecursivePartial<T> = {
	[P in keyof T]?: T[P] extends (infer U)[]
		? RecursivePartial<U>[]
		: T[P] extends object | undefined
			? RecursivePartial<T[P]>
			: T[P];
};

interface EchoProvisionerResponses {
	// parse is for observing any Terraform variables
	parse?: RecursivePartial<Response>[];
	// plan occurs when the template is imported
	plan?: RecursivePartial<Response>[];
	// apply occurs when the workspace is built
	apply?: RecursivePartial<Response>[];
}

/**
 * createTemplateVersionTar consumes a series of echo provisioner protobufs and
 * converts it into an uploadable tar file.
 */
const createTemplateVersionTar = async (
	responses?: EchoProvisionerResponses,
): Promise<Buffer> => {
	if (!responses) {
		responses = {};
	}
	if (!responses.parse) {
		responses.parse = [
			{
				parse: {},
			},
		];
	}
	if (!responses.apply) {
		responses.apply = [
			{
				apply: {},
			},
		];
	}
	if (!responses.plan) {
		responses.plan = responses.apply.map((response) => {
			if (response.log) {
				return response;
			}
			return {
				plan: {
					error: response.apply?.error ?? "",
					resources: response.apply?.resources ?? [],
					parameters: response.apply?.parameters ?? [],
					externalAuthProviders: response.apply?.externalAuthProviders ?? [],
					timings: response.apply?.timings ?? [],
					presets: [],
				},
			};
		});
	}

	const tar = new TarWriter();
	responses.parse.forEach((response, index) => {
		response.parse = {
			templateVariables: [],
			error: "",
			readme: new Uint8Array(),
			workspaceTags: {},
			...response.parse,
		} as ParseComplete;
		tar.addFile(
			`${index}.parse.protobuf`,
			Response.encode(response as Response).finish(),
		);
	});

	const fillResource = (resource: RecursivePartial<Resource>) => {
		if (resource.agents) {
			resource.agents = resource.agents?.map(
				(agent: RecursivePartial<Agent>) => {
					if (agent.apps) {
						agent.apps = agent.apps.map((app) => {
							return {
								command: "",
								displayName: "example",
								external: false,
								icon: "",
								sharingLevel: AppSharingLevel.PUBLIC,
								slug: "example",
								subdomain: false,
								url: "",
								...app,
							} as App;
						});
					}
					const agentResource = {
						apps: [],
						architecture: "amd64",
						connectionTimeoutSeconds: 300,
						directory: "",
						env: {},
						id: randomUUID(),
						metadata: [],
						extraEnvs: [],
						scripts: [],
						motdFile: "",
						name: "dev",
						operatingSystem: "linux",
						shutdownScript: "",
						shutdownScriptTimeoutSeconds: 0,
						startupScript: "",
						startupScriptBehavior: "",
						startupScriptTimeoutSeconds: 300,
						troubleshootingUrl: "",
						token: randomUUID(),
						...agent,
					} as Agent;

					try {
						Agent.encode(agentResource);
					} catch (e) {
						let m = "Error: agentResource encode failed, missing defaults?";
						if (e instanceof Error) {
							if (!e.stack?.includes(e.message)) {
								m += `\n${e.name}: ${e.message}`;
							}
							m += `\n${e.stack}`;
						} else {
							m += `\n${e}`;
						}
						throw new Error(m);
					}

					return agentResource;
				},
			);
		}
		return {
			agents: [],
			dailyCost: 0,
			hide: false,
			icon: "",
			instanceType: "",
			metadata: [],
			name: "dev",
			type: "echo",
			modulePath: "",
			...resource,
		} as Resource;
	};

	responses.apply.forEach((response, index) => {
		response.apply = {
			error: "",
			state: new Uint8Array(),
			resources: [],
			parameters: [],
			externalAuthProviders: [],
			timings: [],
			...response.apply,
		} as ApplyComplete;
		response.apply.resources = response.apply.resources?.map(fillResource);

		tar.addFile(
			`${index}.apply.protobuf`,
			Response.encode(response as Response).finish(),
		);
	});
	responses.plan.forEach((response, index) => {
		response.plan = {
			error: "",
			resources: [],
			parameters: [],
			externalAuthProviders: [],
			timings: [],
			modules: [],
			presets: [],
			...response.plan,
		} as PlanComplete;
		response.plan.resources = response.plan.resources?.map(fillResource);

		tar.addFile(
			`${index}.plan.protobuf`,
			Response.encode(response as Response).finish(),
		);
	});
	const tarFile = await tar.write();
	return Buffer.from(
		tarFile instanceof Blob ? await tarFile.arrayBuffer() : tarFile,
	);
};

export const randomName = (annotation?: string) => {
	const base = randomUUID().slice(0, 8);
	return annotation ? `${annotation}-${base}` : base;
};

/**
 * Awaiter is a helper that allows you to wait for a callback to be called. It
 * is useful for waiting for events to occur.
 */
export class Awaiter {
	private promise: Promise<void>;
	private callback?: () => void;

	constructor() {
		this.promise = new Promise((r) => {
			this.callback = r;
		});
	}

	public done(): void {
		if (this.callback) {
			this.callback();
		} else {
			this.promise = Promise.resolve();
		}
	}

	public wait(): Promise<void> {
		return this.promise;
	}
}

export const createServer = async (
	port: number,
): Promise<ReturnType<typeof express>> => {
	await waitForPort(port); // Wait until the port is available

	const e = express();
	// We need to specify the local IP address as the web server
	// tends to fail with IPv6 related error:
	// listen EADDRINUSE: address already in use :::50516
	await new Promise<void>((r) => e.listen(port, "0.0.0.0", r));
	return e;
};

async function waitForPort(
	port: number,
	host = "0.0.0.0",
	timeout = 60_000,
): Promise<void> {
	const start = Date.now();
	while (Date.now() - start < timeout) {
		const available = await isPortAvailable(port, host);
		if (available) {
			return;
		}
		console.warn(`${host}:${port} is in use, checking again in 1s`);
		await new Promise((resolve) => setTimeout(resolve, 1000)); // Wait 1 second before retrying
	}
	throw new Error(
		`Timeout: port ${port} is still in use after ${timeout / 1000} seconds.`,
	);
}

function isPortAvailable(port: number, host = "0.0.0.0"): Promise<boolean> {
	return new Promise((resolve) => {
		const probe = net
			.createServer()
			.once("error", (err: NodeJS.ErrnoException) => {
				if (err.code === "EADDRINUSE") {
					resolve(false); // port is in use
				} else {
					resolve(false); // some other error occurred
				}
			})
			.once("listening", () => {
				probe.close();
				resolve(true); // port is available
			})
			.listen(port, host);
	});
}

export const findSessionToken = async (page: Page): Promise<string> => {
	const cookies = await page.context().cookies();
	const sessionCookie = cookies.find((c) => c.name === "coder_session_token");
	if (!sessionCookie) {
		throw new Error("session token not found");
	}
	return sessionCookie.value;
};

export const echoResponsesWithParameters = (
	richParameters: RichParameter[],
): EchoProvisionerResponses => {
	return {
		parse: [
			{
				parse: {},
			},
		],
		plan: [
			{
				plan: {
					parameters: richParameters,
				},
			},
		],
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
	};
};

export const echoResponsesWithExternalAuth = (
	providers: ExternalAuthProviderResource[],
): EchoProvisionerResponses => {
	return {
		parse: [
			{
				parse: {},
			},
		],
		plan: [
			{
				plan: {
					externalAuthProviders: providers,
				},
			},
		],
		apply: [
			{
				apply: {
					externalAuthProviders: providers,
					resources: [
						{
							name: "example",
						},
					],
				},
			},
		],
	};
};

export const fillParameters = async (
	page: Page,
	richParameters: RichParameter[] = [],
	buildParameters: WorkspaceBuildParameter[] = [],
) => {
	for (const buildParameter of buildParameters) {
		const richParameter = richParameters.find(
			(richParam) => richParam.name === buildParameter.name,
		);
		if (!richParameter) {
			throw new Error(
				"build parameter is expected to be present in rich parameter schema",
			);
		}

		const parameterLabel = await page.waitForSelector(
			`[data-testid='parameter-field-${richParameter.name}']`,
			{ state: "visible" },
		);

		if (richParameter.type === "bool") {
			const parameterField = await parameterLabel.waitForSelector(
				`[data-testid='parameter-field-bool'] .MuiRadio-root input[value='${buildParameter.value}']`,
			);
			await parameterField.click();
		} else if (richParameter.options.length > 0) {
			const parameterField = await parameterLabel.waitForSelector(
				`[data-testid='parameter-field-options'] .MuiRadio-root input[value='${buildParameter.value}']`,
			);
			await parameterField.click();
		} else if (richParameter.type === "list(string)") {
			throw new Error("not implemented yet"); // FIXME
		} else {
			// text or number
			const parameterField = await parameterLabel.waitForSelector(
				"[data-testid='parameter-field-text'] input",
			);
			await parameterField.fill(buildParameter.value);
		}
	}
};

export const updateTemplate = async (
	page: Page,
	organization: string,
	templateName: string,
	responses?: EchoProvisionerResponses,
) => {
	const tarball = await createTemplateVersionTar(responses);

	const sessionToken = await findSessionToken(page);
	const child = spawn(
		coderBinary,
		[
			"templates",
			"push",
			"--test.provisioner",
			"echo",
			"-y",
			"-d",
			"-",
			"-O",
			organization,
			templateName,
		],
		{
			env: {
				...process.env,
				CODER_SESSION_TOKEN: sessionToken,
				CODER_URL: `http://localhost:${coderPort}`,
			},
		},
	);

	const uploaded = new Awaiter();

	child.on("exit", (code) => {
		if (code === 0) {
			uploaded.done();
			return;
		}

		throw new Error(`coder templates push failed with code ${code}`);
	});

	child.stdin.write(tarball);
	child.stdin.end();

	await uploaded.wait();
};

export const updateTemplateSettings = async (
	page: Page,
	templateName: string,
	templateSettingValues: Pick<
		UpdateTemplateMeta,
		"name" | "display_name" | "description" | "deprecation_message"
	>,
) => {
	await page.goto(`/templates/${templateName}/settings`, {
		waitUntil: "domcontentloaded",
	});

	for (const [key, value] of Object.entries(templateSettingValues)) {
		// Skip max_port_share_level for now since the frontend is not yet able to handle it
		if (key === "max_port_share_level") {
			continue;
		}
		const labelText = capitalize(key).replace("_", " ");
		await page.getByLabel(labelText, { exact: true }).fill(value);
	}

	await page.getByRole("button", { name: /save/i }).click();

	const name = templateSettingValues.name ?? templateName;
	await expectUrl(page).toHavePathNameEndingWith(`/${name}`);
};

export const updateWorkspace = async (
	page: Page,
	workspaceName: string,
	richParameters: RichParameter[] = [],
	buildParameters: WorkspaceBuildParameter[] = [],
) => {
	const user = currentUser(page);
	await page.goto(`/@${user.username}/${workspaceName}`, {
		waitUntil: "domcontentloaded",
	});

	await page.getByTestId("workspace-update-button").click();
	await page.getByTestId("confirm-button").click();

	await fillParameters(page, richParameters, buildParameters);
	await page.getByRole("button", { name: /update parameters/i }).click();

	await page.waitForSelector("*[data-testid='build-status'] >> text=Running", {
		state: "visible",
	});
};

export const updateWorkspaceParameters = async (
	page: Page,
	workspaceName: string,
	richParameters: RichParameter[] = [],
	buildParameters: WorkspaceBuildParameter[] = [],
) => {
	const user = currentUser(page);
	await page.goto(`/@${user.username}/${workspaceName}/settings/parameters`, {
		waitUntil: "domcontentloaded",
	});

	await fillParameters(page, richParameters, buildParameters);
	await page.getByRole("button", { name: /submit and restart/i }).click();

	await page.waitForSelector("*[data-testid='build-status'] >> text=Running", {
		state: "visible",
	});
};

export async function openTerminalWindow(
	page: Page,
	context: BrowserContext,
	workspaceName: string,
	agentName = "dev",
): Promise<Page> {
	// Wait for the web terminal to open in a new tab
	const pagePromise = context.waitForEvent("page");
	await page.getByTestId("terminal").click({ timeout: 60_000 });
	const terminal = await pagePromise;
	await terminal.waitForLoadState("domcontentloaded");

	// Specify that the shell should be `bash`, to prevent inheriting a shell that
	// isn't POSIX compatible, such as Fish.
	const user = currentUser(page);
	const commandQuery = `?command=${encodeURIComponent("/usr/bin/env bash")}`;
	await expectUrl(terminal).toHavePathName(
		`/@${user.username}/${workspaceName}.${agentName}/terminal`,
	);
	await terminal.goto(
		`/@${user.username}/${workspaceName}.${agentName}/terminal${commandQuery}`,
	);

	return terminal;
}

type UserValues = {
	name: string;
	username: string;
	email: string;
	password: string;
	roles: string[];
};

export async function createUser(
	page: Page,
	userValues: Partial<UserValues> = {},
): Promise<UserValues> {
	const returnTo = page.url();

	await page.goto("/deployment/users", { waitUntil: "domcontentloaded" });
	await expect(page).toHaveTitle("Users - Coder");

	await page.getByRole("link", { name: "Create user" }).click();
	await expect(page).toHaveTitle("Create User - Coder");

	const username = userValues.username ?? randomName();
	const name = userValues.name ?? username;
	const email = userValues.email ?? `${username}@coder.com`;
	const password = userValues.password || defaultPassword;
	const roles = userValues.roles ?? [];

	await page.getByLabel("Username").fill(username);
	if (name) {
		await page.getByLabel("Full name").fill(name);
	}
	await page.getByLabel("Email").fill(email);
	await page.getByLabel("Login Type").click();
	await page.getByRole("option", { name: "Password", exact: false }).click();
	// Using input[name=password] due to the select element utilizing 'password'
	// as the label for the currently active option.
	const passwordField = page.locator("input[name=password]");
	await passwordField.fill(password);
	await page.getByRole("button", { name: /save/i }).click();
	await expect(page.getByText("Successfully created user.")).toBeVisible();

	await expect(page).toHaveTitle("Users - Coder");
	const addedRow = page.locator("tr", { hasText: email });
	await expect(addedRow).toBeVisible();

	// Give them a role
	await addedRow.getByLabel("Edit user roles").click();
	for (const role of roles) {
		await page.getByRole("group").getByText(role, { exact: true }).click();
	}
	await page.mouse.click(10, 10); // close the popover by clicking outside of it

	await page.goto(returnTo, { waitUntil: "domcontentloaded" });
	return { name, username, email, password, roles };
}

export async function createOrganization(page: Page): Promise<{
	name: string;
	displayName: string;
	description: string;
}> {
	// Create a new organization to test
	await page.goto("/organizations/new", { waitUntil: "domcontentloaded" });
	const name = randomName();
	await page.getByLabel("Slug").fill(name);
	const displayName = `Org ${name}`;
	await page.getByLabel("Display name").fill(displayName);
	const description = `Org description ${name}`;
	await page.getByLabel("Description").fill(description);
	await page.getByLabel("Icon", { exact: true }).fill("/emojis/1f957.png");
	await page.getByRole("button", { name: /save/i }).click();

	await expectUrl(page).toHavePathName(`/organizations/${name}`);
	await expect(page.getByText("Organization created.")).toBeVisible();

	return { name, displayName, description };
}

/**
 * @param organization organization name
 * @param user user email or username
 */
export async function addUserToOrganization(
	page: Page,
	organization: string,
	user: string,
	roles: string[] = [],
): Promise<void> {
	await page.goto(`/organizations/${organization}`, {
		waitUntil: "domcontentloaded",
	});

	await page.getByPlaceholder("User email or username").fill(user);
	await page.getByRole("option", { name: user }).click();
	await page.getByRole("button", { name: "Add user" }).click();
	const addedRow = page.locator("tr", { hasText: user });
	await expect(addedRow).toBeVisible();

	await addedRow.getByLabel("Edit user roles").click();
	for (const role of roles) {
		await page.getByText(role).click();
	}
	await page.mouse.click(10, 10); // close the popover by clicking outside of it
}
