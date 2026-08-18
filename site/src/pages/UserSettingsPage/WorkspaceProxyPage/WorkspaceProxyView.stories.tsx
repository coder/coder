import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, within } from "storybook/test";
import {
	MockHealthyWildWorkspaceProxy,
	MockPermissions,
	MockPrimaryWorkspaceProxy,
	MockProxyLatencies,
	MockWorkspaceProxies,
	mockApiError,
} from "#/testHelpers/entities";
import { WorkspaceProxyView } from "./WorkspaceProxyView";

const meta: Meta<typeof WorkspaceProxyView> = {
	title: "pages/UserSettingsPage/WorkspaceProxyView",
	component: WorkspaceProxyView,
	args: {
		showPaywall: false,
		permissions: MockPermissions,
	},
};

export default meta;
type Story = StoryObj<typeof WorkspaceProxyView>;

export const PrimarySelected: Story = {
	args: {
		isLoading: false,
		hasLoaded: true,
		proxies: MockWorkspaceProxies,
		proxyLatencies: MockProxyLatencies,
		preferredProxy: MockPrimaryWorkspaceProxy,
	},
};

export const Example: Story = {
	args: {
		isLoading: false,
		hasLoaded: true,
		proxies: MockWorkspaceProxies,
		proxyLatencies: MockProxyLatencies,
		preferredProxy: MockHealthyWildWorkspaceProxy,
	},
};

export const Paywall: Story = {
	args: {
		...Example.args,
		showPaywall: true,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);

		const cta = canvas.getByRole("link", { name: "Start trial for free" });
		await expect(cta).toHaveAttribute("href", "/deployment/premium");
	},
};

export const PaywallWithoutLicenseAccess: Story = {
	args: {
		...Paywall.args,
		permissions: { ...MockPermissions, viewAllLicenses: false },
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);

		await expect(
			canvas.getByText(/contact your deployment administrator/i),
		).toBeVisible();
		await expect(
			canvas.queryByRole("link", { name: "Start trial for free" }),
		).not.toBeInTheDocument();
	},
};

export const Loading: Story = {
	args: {
		...Example.args,
		isLoading: true,
		hasLoaded: false,
	},
};

export const Empty: Story = {
	args: {
		...Example.args,
		proxies: [],
	},
};

export const WithProxiesError: Story = {
	args: {
		...Example.args,
		hasLoaded: false,
		getWorkspaceProxiesError: mockApiError({
			message: "Failed to get proxies.",
		}),
	},
};

export const WithSelectProxyError: Story = {
	args: {
		...Example.args,
		hasLoaded: false,
		selectProxyError: mockApiError({
			message: "Failed to select proxy.",
		}),
	},
};
