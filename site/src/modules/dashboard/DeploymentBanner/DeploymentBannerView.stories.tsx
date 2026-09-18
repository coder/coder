import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, screen, userEvent, waitFor, within } from "storybook/test";
import type {
	DeploymentStats,
	SessionCountDeploymentStats,
} from "#/api/typesGenerated";
import {
	DeploymentHealthUnhealthy,
	MockDeploymentStats,
} from "#/testHelpers/entities";
import { DeploymentBannerView } from "./DeploymentBannerView";

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

// Seven apps: four visible, three behind "+3 more", one of them without a
// bundled icon.
const manyApps = withSessionCount({
	...MockDeploymentStats.session_count.apps,
	zed: { count: 7, display_name: "Zed", icon: "/icon/zed.svg" },
	vscodium: { count: 4, display_name: "VSCodium" },
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

// Session count edge cases in one screenshot: no apps, an unrecognized app, a
// long display name, apps without a bundled icon, and the overflow trigger. An
// app renders its name whenever no bundled icon stands in for it.
export const SessionCountVariants: Story = {
	render: () => (
		<div className="grid gap-2">
			<DeploymentBannerView stats={withSessionCount({})} />
			<DeploymentBannerView
				stats={withSessionCount({
					unknown_app: { count: 3, display_name: "unknown_app" },
				})}
			/>
			<DeploymentBannerView
				stats={withSessionCount({
					long_name: {
						count: 1,
						display_name:
							"A workspace application with an intentionally long display name",
					},
				})}
			/>
			<DeploymentBannerView
				stats={withSessionCount({
					vscodium: { count: 4, display_name: "VSCodium" },
					trae: { count: 2, display_name: "Trae" },
				})}
			/>
			<DeploymentBannerView
				stats={withSessionCount({
					offsite_icon: {
						count: 1,
						display_name: "Offsite Icon",
						icon: "https://example.com/icon.svg",
					},
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

// The server caps a report at 64 app names, so the popover must stay usable
// when nearly all of them overflow.
export const OverflowCapped: Story = {
	args: {
		stats: withSessionCount(
			Object.fromEntries(
				Array.from({ length: 65 }, (_, i) => [
					`custom_application_${i}`,
					{ count: 65 - i, display_name: `custom_application_${i}` },
				]),
			),
		),
	},
	play: async ({ canvasElement }) => {
		await userEvent.click(
			within(canvasElement).getByRole("button", { name: "+61 more" }),
		);
	},
};

export const FamilyTotals: Story = {
	play: async ({ canvasElement }) => {
		await userEvent.hover(
			within(canvasElement).getByRole("button", { name: "Active Connections" }),
		);
		// Wait for the tooltip to open before the screenshot.
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
