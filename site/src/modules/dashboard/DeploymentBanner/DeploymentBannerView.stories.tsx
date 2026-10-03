import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, screen, userEvent, waitFor, within } from "storybook/test";
import type {
	AppFamilyName,
	DeploymentStats,
	SessionCountApp,
	SessionCountDeploymentStats,
} from "#/api/typesGenerated";
import {
	DeploymentHealthUnhealthy,
	MockDeploymentStats,
} from "#/testHelpers/entities";
import { DeploymentBannerView } from "./DeploymentBannerView";

const app = (
	count: number,
	display_name: string,
	family: AppFamilyName = "unknown",
	icon?: string,
): SessionCountApp => ({ count, display_name, family, icon });

/** Zeroes the deprecated family totals, since the banner reads apps. */
const withSessionCount = (
	apps: SessionCountDeploymentStats["apps"],
): DeploymentStats => ({
	...MockDeploymentStats,
	session_count: {
		vscode: 0,
		jetbrains: 0,
		ssh: 0,
		reconnecting_pty: 0,
		apps,
	},
});

const otherApps = withSessionCount({
	...MockDeploymentStats.session_count.apps,
	unknown_app: app(3, "unknown_app"),
	long_name: app(
		1,
		"A workspace application with an intentionally long display name",
	),
	offsite_icon: app(
		1,
		"Offsite Icon",
		"unknown",
		"https://example.com/icon.svg",
	),
	sftp: app(2, "SFTP", "sftp", "/icon/terminal.svg"),
	...Object.fromEntries(
		Array.from({ length: 20 }, (_, i) => [
			`custom_${i}`,
			app(1, `custom_${i}`),
		]),
	),
});

const meta: Meta<typeof DeploymentBannerView> = {
	title: "modules/dashboard/DeploymentBannerView",
	component: DeploymentBannerView,
	args: {
		stats: MockDeploymentStats,
	},
};

export default meta;
type Story = StoryObj<typeof DeploymentBannerView>;

export const Example: Story = {};

export const Loading: Story = {
	args: { stats: undefined },
	play: async ({ canvasElement }) => {
		within(canvasElement)
			.getByRole("button", {
				name: "Visual Studio Code: - loading active connections",
			})
			.focus();
		await waitFor(() => screen.getByRole("tooltip"));
	},
};

export const NoActiveConnections: Story = {
	args: { stats: withSessionCount({}) },
	play: async ({ canvasElement }) => {
		await userEvent.hover(
			within(canvasElement).getByRole("button", {
				name: "Visual Studio Code: 0 active connections",
			}),
		);
		await waitFor(() => screen.getByRole("tooltip"));
	},
};

/** VS Code and its forks, as a real deployment reports them. */
export const VSCodeForks: Story = {
	args: {
		stats: withSessionCount({
			...MockDeploymentStats.session_count.apps,
			vscode_insiders: app(
				12,
				"VS Code Insiders",
				"vscode",
				"/icon/code-insiders.svg",
			),
			antigravity: app(9, "Antigravity", "vscode", "/icon/antigravity.svg"),
		}),
	},
	play: async ({ canvasElement }) => {
		await userEvent.hover(
			within(canvasElement).getByRole("button", {
				name: "Visual Studio Code: 173 active connections",
			}),
		);
		await waitFor(() => screen.getByRole("tooltip"));
	},
};

/** Long names, an offsite icon, a family without a slot, and a scrolling list. */
export const OtherApps: Story = {
	args: { stats: otherApps },
	play: async ({ canvasElement }) => {
		await userEvent.hover(
			within(canvasElement).getByRole("button", {
				name: "Other: 27 active connections",
			}),
		);
		await waitFor(() => screen.getByRole("tooltip"));
	},
};

export const WithHealthIssues: Story = {
	args: {
		health: DeploymentHealthUnhealthy,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const trigger = canvas.getByTestId("deployment-health-trigger");
		await userEvent.hover(trigger);
		await waitFor(() =>
			expect(screen.getByRole("tooltip")).toBeInTheDocument(),
		);
	},
};

export const WithDismissedHealthIssues: Story = {
	args: {
		health: {
			...DeploymentHealthUnhealthy,
			workspace_proxy: {
				...DeploymentHealthUnhealthy.workspace_proxy,
				dismissed: true,
			},
		},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const trigger = canvas.getByTestId("deployment-health-trigger");
		await userEvent.hover(trigger);
		await waitFor(() =>
			expect(screen.getByRole("tooltip")).toBeInTheDocument(),
		);
	},
};
