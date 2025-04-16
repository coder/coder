import type { Meta, StoryObj } from "@storybook/react";
import { getAuthorizationKey } from "api/queries/authCheck";
import { workspaceByOwnerAndNameKey } from "api/queries/workspaces";
import type { Workspace, WorkspaceAgentLifecycle } from "api/typesGenerated";
import { AuthProvider } from "contexts/auth/AuthProvider";
import { RequireAuth } from "contexts/auth/RequireAuth";
import { permissionChecks } from "modules/permissions";
import {
	reactRouterOutlet,
	reactRouterParameters,
} from "storybook-addon-remix-react-router";
import {
	MockAppearanceConfig,
	MockAuthMethodsAll,
	MockBuildInfo,
	MockDefaultOrganization,
	MockDeploymentConfig,
	MockEntitlements,
	MockExperiments,
	MockUser,
	MockUserAppearanceSettings,
	MockWorkspace,
	MockWorkspaceAgent,
} from "testHelpers/entities";
import { withWebSocket } from "testHelpers/storybook";
import TerminalPage from "./TerminalPage";

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
			{ key: ["me"], data: MockUser },
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
			{
				key: ["deployment", "config"],
				data: {
					...MockDeploymentConfig,
					config: {
						...MockDeploymentConfig.config,
						web_terminal_renderer: "canvas",
					},
				},
			},
		],
		chromatic: {
			diffThreshold: 0.3,
		},
	},
	decorators: [
		(Story) => (
			<AuthProvider>
				<Story />
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
		// Forcing smaller viewport to make it easier to identify the issue
		viewport: {
			defaultViewport: "terminal",
		},
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
};
