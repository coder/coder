import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, within } from "storybook/test";
import {
	MockBuildInfo,
	MockPermissions,
	MockProvisioner,
	MockProvisionerWithTags,
	MockUserProvisioner,
	mockApiError,
} from "#/testHelpers/entities";
import { docs } from "#/utils/docs";
import { OrganizationProvisionersPageView } from "./OrganizationProvisionersPageView";

const meta: Meta<typeof OrganizationProvisionersPageView> = {
	title: "pages/OrganizationProvisionersPage",
	component: OrganizationProvisionersPageView,
	args: {
		buildVersion: MockBuildInfo.version,
		provisioners: [
			MockProvisioner,
			{
				...MockUserProvisioner,
				status: "busy",
			},
			{
				...MockProvisionerWithTags,
				version: "0.0.0",
			},
			{
				...MockUserProvisioner,
				status: "offline",
			},
		],
		filter: {
			ids: "",
			offline: true,
		},
		permissions: MockPermissions,
	},
};

export default meta;
type Story = StoryObj<typeof OrganizationProvisionersPageView>;

export const Loaded: Story = {};

export const Loading: Story = {
	args: {
		provisioners: undefined,
	},
};

export const Empty: Story = {
	args: {
		provisioners: [],
	},
};

export const WithError: Story = {
	args: {
		provisioners: undefined,
		error: mockApiError({
			message: "Fern is mad",
			detail: "Frieren slept in and didn't get groceries",
		}),
	},
};

export const Paywall: Story = {
	args: {
		provisioners: undefined,
		showPaywall: true,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);

		const cta = canvas.getByRole("link", { name: "Start trial for free" });
		await expect(cta).toHaveAttribute("href", "/deployment/premium");
		await expect(
			canvas.getByRole("link", { name: /Read the docs/ }),
		).toHaveAttribute("href", docs("/admin/provisioners"));
	},
};

export const PaywallWithoutLicenseAccess: Story = {
	args: {
		provisioners: undefined,
		showPaywall: true,
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

export const FilterByID: Story = {
	args: {
		provisioners: [MockProvisioner],
		filter: {
			ids: MockProvisioner.id,
			offline: true,
		},
	},
};

export const FilterByOffline: Story = {
	args: {
		provisioners: [MockProvisioner],
		filter: {
			ids: "",
			offline: false,
		},
	},
};
