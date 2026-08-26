import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, within } from "storybook/test";
import {
	type ProvisionerKeyDaemons,
	ProvisionerKeyIDBuiltIn,
	ProvisionerKeyIDPSK,
	ProvisionerKeyIDUserAuth,
} from "#/api/typesGenerated";
import {
	MockPermissions,
	MockProvisioner,
	MockProvisionerKey,
	mockApiError,
} from "#/testHelpers/entities";
import { docs } from "#/utils/docs";
import { OrganizationProvisionerKeysPageView } from "./OrganizationProvisionerKeysPageView";

const mockProvisionerKeyDaemons: ProvisionerKeyDaemons[] = [
	{
		key: {
			...MockProvisionerKey,
		},
		daemons: [
			{
				...MockProvisioner,
				name: "Test Provisioner 1",
				id: "daemon-1",
			},
			{
				...MockProvisioner,
				name: "Test Provisioner 2",
				id: "daemon-2",
			},
		],
	},
	{
		key: {
			...MockProvisionerKey,
			name: "no-daemons",
		},
		daemons: [],
	},
	// Built-in provisioners, user-auth, and PSK keys are not shown here.
	{
		key: {
			...MockProvisionerKey,
			id: ProvisionerKeyIDBuiltIn,
			name: "built-in",
		},
		daemons: [],
	},
	{
		key: {
			...MockProvisionerKey,
			id: ProvisionerKeyIDUserAuth,
			name: "user-auth",
		},
		daemons: [],
	},
	{
		key: {
			...MockProvisionerKey,
			id: ProvisionerKeyIDPSK,
			name: "PSK",
		},
		daemons: [],
	},
];

const meta: Meta<typeof OrganizationProvisionerKeysPageView> = {
	title: "pages/OrganizationProvisionerKeysPage",
	component: OrganizationProvisionerKeysPageView,
	args: {
		error: undefined,
		provisionerKeyDaemons: mockProvisionerKeyDaemons,
		permissions: MockPermissions,
		onRetry: () => {},
	},
};

export default meta;
type Story = StoryObj<typeof OrganizationProvisionerKeysPageView>;

export const Default: Story = {
	args: {
		error: undefined,
		provisionerKeyDaemons: mockProvisionerKeyDaemons,
		onRetry: () => {},
		showPaywall: false,
	},
};

export const Paywalled: Story = {
	...Default,
	args: {
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

export const PaywalledWithoutLicenseAccess: Story = {
	...Default,
	args: {
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

export const Loading: Story = {
	...Default,
	args: {
		provisionerKeyDaemons: undefined,
	},
};

export const Empty: Story = {
	...Default,
	args: {
		provisionerKeyDaemons: [],
	},
};

export const WithError: Story = {
	...Default,
	args: {
		provisionerKeyDaemons: undefined,
		error: mockApiError({
			message: "Error loading provisioner keys",
			detail: "Something went wrong. This is an unhelpful error message.",
		}),
	},
};
