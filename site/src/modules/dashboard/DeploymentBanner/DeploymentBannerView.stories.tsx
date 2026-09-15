import type { Meta, StoryObj } from "@storybook/react-vite";
import { fireEvent, userEvent, within } from "storybook/test";
import {
	DeploymentHealthUnhealthy,
	MockDeploymentStats,
} from "#/testHelpers/entities";
import { DeploymentBannerView } from "./DeploymentBannerView";

const statsWithUnknownApp = {
	...MockDeploymentStats,
	session_count: {
		...MockDeploymentStats.session_count,
		vscode: 0,
		jetbrains: 0,
		ssh: 0,
		reconnecting_pty: 0,
		session_counts: {
			unknown_app: 3,
		},
		apps: {},
	},
};

const statsWithNoActiveConnections = {
	...MockDeploymentStats,
	session_count: {
		...MockDeploymentStats.session_count,
		vscode: 0,
		jetbrains: 0,
		ssh: 0,
		reconnecting_pty: 0,
		session_counts: {},
		apps: {},
	},
};

const statsWithManyApps = {
	...MockDeploymentStats,
	session_count: {
		...MockDeploymentStats.session_count,
		ssh: 39,
		session_counts: {
			cursor: 24,
			jetbrains: 5,
			ssh: 32,
			vscode: 128,
			reconnecting_pty: 15,
			zero_count: 0,
			negative_count: -1,
			zed: 7,
		},
		apps: {
			...MockDeploymentStats.session_count.apps,
			zed: {
				display_name: "Zed",
				icon: "/icon/zed.svg",
			},
		},
	},
};

const statsWithLongAppNames = {
	...MockDeploymentStats,
	session_count: {
		...MockDeploymentStats.session_count,
		session_counts: {
			long_name: 1,
		},
		apps: {
			long_name: {
				display_name:
					"A workspace application with an intentionally long display name",
			},
		},
	},
};

const statsWithAppWithoutIcon = {
	...MockDeploymentStats,
	session_count: {
		...MockDeploymentStats.session_count,
		session_counts: {
			vscodium: 4,
			trae: 2,
		},
		apps: {
			vscodium: {
				display_name: "VSCodium",
			},
			trae: {
				display_name: "Trae",
			},
		},
	},
};
const statsWithBrokenIcon = {
	...MockDeploymentStats,
	session_count: {
		...MockDeploymentStats.session_count,
		session_counts: {
			cursor: 24,
		},
		apps: {
			cursor: {
				display_name: "Cursor",
				icon: "/icon/does-not-exist.svg",
			},
		},
	},
};

const meta: Meta<typeof DeploymentBannerView> = {
	title: "modules/dashboard/DeploymentBannerView",
	component: DeploymentBannerView,
	args: {
		stats: MockDeploymentStats,
	},
};

export default meta;
type Story = StoryObj<typeof DeploymentBannerView>;

export const Cursor: Story = {};

export const Loading: Story = {
	args: {
		stats: undefined,
	},
};

export const UnknownApp: Story = {
	args: {
		stats: statsWithUnknownApp,
	},
};

export const NoActiveConnections: Story = {
	args: {
		stats: statsWithNoActiveConnections,
	},
};

export const ManyApps: Story = {
	args: {
		stats: statsWithManyApps,
	},
};

export const FourApps: Story = {
	args: {
		stats: {
			...MockDeploymentStats,
			session_count: {
				...MockDeploymentStats.session_count,
				jetbrains: 0,
				session_counts: {
					vscode: 128,
					ssh: 32,
					cursor: 24,
					reconnecting_pty: 15,
				},
			},
		},
	},
};

export const OverflowOpen: Story = {
	...ManyApps,
	play: async ({ canvasElement }) => {
		await userEvent.click(
			within(canvasElement).getByRole("button", { name: "+2 more" }),
		);
	},
};

export const OverflowKeyboard: Story = {
	play: async ({ canvasElement }) => {
		within(canvasElement).getByRole("button", { name: "+1 more" }).focus();
		await userEvent.keyboard("{Enter}");
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
	},
};

export const FamilyTotalsLight: Story = {
	...FamilyTotals,
	parameters: { themes: { themeOverride: "light" } },
};

export const FamilyTotalsLoading: Story = {
	...Loading,
	play: FamilyTotals.play,
};

export const FamilyTotalsEmpty: Story = {
	...NoActiveConnections,
	play: FamilyTotals.play,
};

export const OverflowScrolledAway: Story = {
	...OverflowNarrow,
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const trigger = canvas.getByRole("button", { name: "+2 more" });
		trigger.scrollIntoView({ behavior: "instant", inline: "center" });
		await userEvent.click(trigger);
		canvas
			.getByRole("link", { name: "15" })
			.scrollIntoView({ behavior: "instant", inline: "start" });
	},
};

export const FamilyTotalsKeyboard: Story = {
	play: ({ canvasElement }) => {
		within(canvasElement)
			.getByRole("button", { name: "Active Connections" })
			.focus();
	},
};

export const UnknownAppFamilyTotals: Story = {
	...UnknownApp,
	play: FamilyTotals.play,
};

export const TiedCounts: Story = {
	args: {
		stats: {
			...MockDeploymentStats,
			session_count: {
				vscode: 0,
				jetbrains: 0,
				ssh: 0,
				reconnecting_pty: 0,
				session_counts: {
					zulu: 2,
					echo: 1,
					delta: 1,
					charlie: 1,
					bravo: 1,
					alpha: 1,
				},
				apps: {},
			},
		},
	},
	play: OverflowOpen.play,
};

export const LargeOverflow: Story = {
	args: {
		stats: {
			...MockDeploymentStats,
			session_count: {
				vscode: 0,
				jetbrains: 0,
				ssh: 0,
				reconnecting_pty: 0,
				session_counts: Object.fromEntries(
					Array.from({ length: 65 }, (_, i) => [
						`custom_application_${i}`,
						65 - i,
					]),
				),
				apps: {},
			},
		},
	},
	play: async ({ canvasElement }) => {
		await userEvent.click(
			within(canvasElement).getByRole("button", { name: "+61 more" }),
		);
	},
};

export const LongAppNames: Story = {
	args: {
		stats: statsWithLongAppNames,
	},
};

export const KnownAppsWithoutIcons: Story = {
	args: {
		stats: statsWithAppWithoutIcon,
	},
};

export const BrokenAppIcon: Story = {
	args: {
		stats: statsWithBrokenIcon,
	},
	play: async ({ canvasElement }) => {
		fireEvent.error(
			within(canvasElement).getByRole("img", { name: "Cursor icon" }),
		);
	},
};

export const CursorLight: Story = {
	parameters: { themes: { themeOverride: "light" } },
};

export const WithHealthIssues: Story = {
	args: {
		health: DeploymentHealthUnhealthy,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.hover(canvas.getByTestId("deployment-health-trigger"));
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
		await userEvent.hover(canvas.getByTestId("deployment-health-trigger"));
	},
};
