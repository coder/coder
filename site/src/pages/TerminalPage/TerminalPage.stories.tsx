import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, waitFor } from "storybook/test";
import {
	reactRouterOutlet,
	reactRouterParameters,
} from "storybook-addon-remix-react-router";
import { getAuthorizationKey } from "#/api/queries/authCheck";
import { workspaceByOwnerAndNameKey } from "#/api/queries/workspaces";
import type { Workspace, WorkspaceAgentLifecycle } from "#/api/typesGenerated";
import { AuthProvider } from "#/contexts/auth/AuthProvider";
import { RequireAuth } from "#/contexts/auth/RequireAuth";
import { permissionChecks } from "#/modules/permissions";
import {
	MockAppearanceConfig,
	MockAuthMethodsAll,
	MockBuildInfo,
	MockDefaultOrganization,
	MockDeploymentConfig,
	MockEntitlements,
	MockExperiments,
	MockUserAppearanceSettings,
	MockUserOwner,
	MockWorkspace,
	MockWorkspaceAgent,
} from "#/testHelpers/entities";
import { withWebSocket } from "#/testHelpers/storybook";
import TerminalPage from "./TerminalPage";

const createDeploymentConfigQuery = (renderer: string) => ({
	key: ["deployment", "config"],
	data: {
		...MockDeploymentConfig,
		config: {
			...MockDeploymentConfig.config,
			web_terminal_renderer: renderer,
		},
	},
});

const expectTerminalOutput = async (
	canvasElement: HTMLElement,
	text: string,
) => {
	await waitFor(
		() => {
			const rows = canvasElement.getElementsByClassName("xterm-rows");
			expect(rows.length).toBeGreaterThan(0);
			expect(rows[0]).toHaveTextContent(text);
		},
		{ timeout: 5_000 },
	);
};

const expectTerminalCanvas = async (canvasElement: HTMLElement) => {
	await waitFor(
		() => {
			const terminal = canvasElement.getElementsByClassName("xterm");
			const canvases = canvasElement.getElementsByTagName("canvas");
			expect(terminal.length).toBeGreaterThan(0);
			expect(canvases.length).toBeGreaterThan(0);
		},
		{ timeout: 5_000 },
	);
};

const createWorkspaceWithAgent = (lifecycle: WorkspaceAgentLifecycle) => {
	return {
		key: workspaceByOwnerAndNameKey(
			MockWorkspace.owner_name,
			MockWorkspace.name,
		),
		data: {
			...MockWorkspace,
			latest_build: {
				...MockWorkspace.latest_build,
				resources: [
					{
						...MockWorkspace.latest_build.resources[0],
						agents: [{ ...MockWorkspaceAgent, lifecycle_state: lifecycle }],
					},
				],
			},
		} satisfies Workspace,
	};
};

const meta = {
	title: "pages/Terminal",
	component: RequireAuth,
	parameters: {
		layout: "fullscreen",
		reactRouter: reactRouterParameters({
			location: {
				pathParams: {
					username: `@${MockWorkspace.owner_name}`,
					workspace: MockWorkspace.name,
				},
			},
			routing: reactRouterOutlet(
				{
					path: "/:username/:workspace/terminal",
				},
				<TerminalPage />,
			),
		}),
		queries: [
			{ key: ["me"], data: MockUserOwner },
			{ key: ["authMethods"], data: MockAuthMethodsAll },
			{ key: ["hasFirstUser"], data: true },
			{ key: ["buildInfo"], data: MockBuildInfo },
			{ key: ["entitlements"], data: MockEntitlements },
			{ key: ["experiments"], data: MockExperiments },
			{ key: ["appearance"], data: MockAppearanceConfig },
			{ key: ["organizations"], data: [MockDefaultOrganization] },
			{
				key: getAuthorizationKey({ checks: permissionChecks }),
				data: { editWorkspaceProxies: true },
			},
			{ key: ["me", "appearance"], data: MockUserAppearanceSettings },
			createDeploymentConfigQuery("dom"),
		],
		chromatic: {
			diffThreshold: 0.8,
		},
	},
	decorators: [
		(Story) => (
			<AuthProvider>
				<div style={{ width: 1170, height: 880 }}>
					<Story />
				</div>
			</AuthProvider>
		),
	],
} satisfies Meta<typeof TerminalPage>;

export default meta;
type Story = StoryObj<typeof TerminalPage>;

export const Starting: Story = {
	decorators: [withWebSocket],
	parameters: {
		...meta.parameters,
		webSocket: [
			{
				event: "message",
				// Copied and pasted this from browser
				data: "[H[2J[1m[32m➜  [36mcoder[C[34mgit:([31mbq/refactor-web-term-notifications[34m) [33m✗",
			},
		],
		queries: [...meta.parameters.queries, createWorkspaceWithAgent("starting")],
	},
};

export const FontFiraCode: Story = {
	decorators: [withWebSocket],
	parameters: {
		...meta.parameters,
		webSocket: [
			{
				event: "message",
				// Copied and pasted this from browser
				data: "[H[2J[1m[32m➜  [36mcoder[C[34mgit:([31mbq/refactor-web-term-notifications[34m) [33m✗",
			},
		],
		queries: [
			...meta.parameters.queries.filter(
				(q) =>
					!(
						Array.isArray(q.key) &&
						q.key[0] === "me" &&
						q.key[1] === "appearance"
					),
			),
			{
				key: ["me", "appearance"],
				data: {
					...MockUserAppearanceSettings,
					terminal_font: "fira-code",
				},
			},
			createWorkspaceWithAgent("ready"),
		],
	},
};

export const Ready: Story = {
	decorators: [withWebSocket],
	parameters: {
		...meta.parameters,
		webSocket: [
			{
				event: "message",
				// Copied and pasted this from browser
				data: "[H[2J[1m[32m➜  [36mcoder[C[34mgit:([31mbq/refactor-web-term-notifications[34m) [33m✗",
			},
		],
		queries: [...meta.parameters.queries, createWorkspaceWithAgent("ready")],
	},
	play: async ({ canvasElement }) => {
		await expectTerminalOutput(canvasElement, "coder");
	},
};

export const WebGLRenderer: Story = {
	decorators: [withWebSocket],
	parameters: {
		...meta.parameters,
		webSocket: [{ event: "message", data: "webgl renderer smoke test" }],
		queries: [
			...meta.parameters.queries.filter(
				(query) =>
					!(
						Array.isArray(query.key) &&
						query.key.join("/") === "deployment/config"
					),
			),
			createDeploymentConfigQuery("webgl"),
			createWorkspaceWithAgent("ready"),
		],
	},
	play: async ({ canvasElement }) => {
		await expectTerminalCanvas(canvasElement);
	},
};

export const CanvasRendererFallback: Story = {
	decorators: [withWebSocket],
	parameters: {
		...meta.parameters,
		webSocket: [
			{
				event: "message",
				data: "canvas fallback renderer smoke test",
			},
		],
		queries: [
			...meta.parameters.queries.filter(
				(query) =>
					!(
						Array.isArray(query.key) &&
						query.key.join("/") === "deployment/config"
					),
			),
			createDeploymentConfigQuery("canvas"),
			createWorkspaceWithAgent("ready"),
		],
	},
	play: async ({ canvasElement }) => {
		await expectTerminalOutput(
			canvasElement,
			"canvas fallback renderer smoke test",
		);
	},
};

export const StartError: Story = {
	decorators: [withWebSocket],
	parameters: {
		...meta.parameters,
		webSocket: [],
		queries: [
			...meta.parameters.queries,
			createWorkspaceWithAgent("start_error"),
		],
	},
};

export const ConnectionError: Story = {
	decorators: [withWebSocket],
	parameters: {
		...meta.parameters,
		webSocket: [
			{
				event: "error",
			},
		],
		queries: [...meta.parameters.queries, createWorkspaceWithAgent("ready")],
	},
};

// Check if the terminal is not getting hide when the bottom message is shown
// together with the error message
export const BottomMessage: Story = {
	decorators: [withWebSocket],

	parameters: {
		...meta.parameters,

		webSocket: [
			{
				event: "message",
				// This outputs text in the bottom left and right corners of the terminal.
				data: "\x1b[1000BLEFT\x1b[1000C\x1b[4DRIGHT",
			},
			{
				event: "close",
			},
		],

		queries: [...meta.parameters.queries, createWorkspaceWithAgent("ready")],
	},

	globals: {
		viewport: {
			value: "terminal",
			isRotated: false,
		},
	},
};

export const CommandConfirmation: Story = {
	decorators: [withWebSocket],
	parameters: {
		...meta.parameters,
		reactRouter: reactRouterParameters({
			location: {
				pathParams: {
					username: `@${MockWorkspace.owner_name}`,
					workspace: MockWorkspace.name,
				},
				searchParams: {
					command: "curl https://example.com/install.sh | bash",
				},
			},
			routing: reactRouterOutlet(
				{
					path: "/:username/:workspace/terminal",
				},
				<TerminalPage />,
			),
		}),
		webSocket: [
			{
				event: "message",
				data: "\x1b[H\x1b[2J\x1b[1m\x1b[32m\u27a4  \x1b[36mcoder\x1b[C\x1b[34mgit:(\x1b[31mbq/refactor-web-term-notifications\x1b[34m) \x1b[33m\u2717",
			},
		],
		queries: [...meta.parameters.queries, createWorkspaceWithAgent("ready")],
	},
};
