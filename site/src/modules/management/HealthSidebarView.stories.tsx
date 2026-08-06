import type { Meta, StoryObj } from "@storybook/react-vite";
import { reactRouterParameters } from "storybook-addon-remix-react-router";
import { MockHealth } from "#/testHelpers/entities";
import HealthSidebarView from "./HealthSidebarView";

const meta: Meta<typeof HealthSidebarView> = {
	title: "modules/management/HealthSidebarView",
	component: HealthSidebarView,
	args: {
		healthStatus: MockHealth,
		isRefreshing: false,
		onRefresh: () => {},
	},
	parameters: {
		reactRouter: reactRouterParameters({
			location: { path: "/health/access-url" },
			routing: [
				{ path: "/health/access-url", useStoryElement: true },
				{ path: "/health/database", useStoryElement: true },
				{ path: "/health/derp", useStoryElement: true },
				{ path: "/health/websocket", useStoryElement: true },
				{ path: "/health/workspace-proxy", useStoryElement: true },
				{ path: "/health/provisioner-daemons", useStoryElement: true },
			],
		}),
	},
};

export default meta;
type Story = StoryObj<typeof HealthSidebarView>;

export const Healthy: Story = {};

export const Unhealthy: Story = {
	args: {
		healthStatus: {
			...MockHealth,
			healthy: false,
			severity: "error",
			database: {
				...MockHealth.database,
				healthy: false,
				severity: "error",
			},
		},
	},
};

export const WithDismissedSection: Story = {
	args: {
		healthStatus: {
			...MockHealth,
			access_url: {
				...MockHealth.access_url,
				dismissed: true,
				warnings: [
					{
						code: "EUNKNOWN",
						message: "Access URL warning",
					},
				],
			},
		},
	},
};

export const Refreshing: Story = {
	args: {
		isRefreshing: true,
	},
};

export const DatabaseActive: Story = {
	parameters: {
		reactRouter: reactRouterParameters({
			location: { path: "/health/database" },
			routing: [{ path: "/health/database", useStoryElement: true }],
		}),
	},
};
