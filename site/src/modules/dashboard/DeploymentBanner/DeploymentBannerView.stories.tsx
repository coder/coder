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
	sessionCount: Partial<SessionCountDeploymentStats>,
): DeploymentStats => ({
	...MockDeploymentStats,
	session_count: {
		vscode: 0,
		jetbrains: 0,
		ssh: 0,
		reconnecting_pty: 0,
		session_counts: {},
		apps: {},
		...sessionCount,
	},
});

// Seven positive apps: four visible, three behind "+3 more", one of them
// without a bundled icon.
const manyApps = withSessionCount({
	vscode: 152,
	jetbrains: 5,
	ssh: 39,
	reconnecting_pty: 15,
	session_counts: {
		...MockDeploymentStats.session_count.session_counts,
		zed: 7,
		vscodium: 4,
		zero_count: 0,
		negative_count: -1,
	},
	apps: {
		...MockDeploymentStats.session_count.apps,
		zed: { display_name: "Zed", icon: "/icon/zed.svg" },
		vscodium: { display_name: "VSCodium" },
	},
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

export const Default: Story = {};

export const DefaultLight: Story = {
	parameters: { themes: { themeOverride: "light" } },
};

export const Loading: Story = {
	args: { stats: undefined },
};

// Static session count variants stacked into one screenshot.
export const SessionCountVariants: Story = {
	render: () => (
		<div className="grid gap-2">
			<DeploymentBannerView stats={withSessionCount({})} />
			<DeploymentBannerView
				stats={withSessionCount({ session_counts: { unknown_app: 3 } })}
			/>
			<DeploymentBannerView
				stats={withSessionCount({
					session_counts: { long_name: 1 },
					apps: {
						long_name: {
							display_name:
								"A workspace application with an intentionally long display name",
						},
					},
				})}
			/>
			<DeploymentBannerView
				stats={withSessionCount({
					vscode: 6,
					session_counts: { vscodium: 4, trae: 2 },
					apps: {
						vscodium: { display_name: "VSCodium" },
						trae: { display_name: "Trae" },
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

export const LargeOverflow: Story = {
	args: {
		stats: withSessionCount({
			session_counts: Object.fromEntries(
				Array.from({ length: 65 }, (_, i) => [
					`custom_application_${i}`,
					65 - i,
				]),
			),
		}),
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
		await waitFor(() =>
			expect(screen.getByRole("tooltip")).toBeInTheDocument(),
		);
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
			derp: {
				...DeploymentHealthUnhealthy.derp,
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
