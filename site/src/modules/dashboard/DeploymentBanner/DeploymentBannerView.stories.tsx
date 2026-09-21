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

// The deprecated family totals are zeroed; the banner reads apps instead.
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

// Seven apps: four visible, three behind "+3 more", two with no icon.
const manyApps = withSessionCount({
	...MockDeploymentStats.session_count.apps,
	zed: app(7, "Zed", "ssh", "/icon/zed.svg"),
	vscodium: app(4, "VSCodium", "vscode"),
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

// Every edge case in one screenshot.
export const SessionCountVariants: Story = {
	render: () => (
		<div className="grid gap-2">
			<DeploymentBannerView stats={withSessionCount({})} />
			<DeploymentBannerView
				stats={withSessionCount({
					unknown_app: app(3, "unknown_app"),
				})}
			/>
			<DeploymentBannerView
				stats={withSessionCount({
					long_name: app(
						1,
						"A workspace application with an intentionally long display name",
					),
				})}
			/>
			<DeploymentBannerView
				stats={withSessionCount({
					vscodium: app(4, "VSCodium", "vscode"),
					trae: app(2, "Trae", "vscode"),
				})}
			/>
			<DeploymentBannerView
				stats={withSessionCount({
					offsite_icon: app(
						1,
						"Offsite Icon",
						"unknown",
						"https://example.com/icon.svg",
					),
				})}
			/>
			<DeploymentBannerView stats={manyApps} />
		</div>
	),
};

export const OverflowOpen: Story = {
	args: { stats: manyApps },
	play: async ({ canvasElement }) => {
		await userEvent.click(
			within(canvasElement).getByRole("button", { name: "+3 more" }),
		);
	},
};

export const OverflowNarrow: Story = {
	...OverflowOpen,
	decorators: [
		(Story) => (
			<div className="w-[390px]">
				<Story />
			</div>
		),
	],
};

export const FamilyTotals: Story = {
	play: async ({ canvasElement }) => {
		await userEvent.hover(
			within(canvasElement).getByRole("button", { name: "Active Connections" }),
		);
		// Let the tooltip open before the screenshot.
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
