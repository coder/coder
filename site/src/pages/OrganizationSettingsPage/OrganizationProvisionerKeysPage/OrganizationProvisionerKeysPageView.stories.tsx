import type { Meta, StoryObj } from "@storybook/react";
import { MockProvisioner, MockProvisionerKey } from "testHelpers/entities";
import { OrganizationProvisionerKeysPageView } from "./OrganizationProvisionerKeysPageView";

const mockProvisionerKeyDaemons = [
	{
		key: {
			...MockProvisionerKey,
		},
		daemons: [
			{
				...MockProvisioner,
			},
		],
	},
];

const meta: Meta<typeof OrganizationProvisionerKeysPageView> = {
	title: "pages/OrganizationProvisionerKeysPage",
	component: OrganizationProvisionerKeysPageView,
	args: {
		error: undefined,
		provisionerKeyDaemons: mockProvisionerKeyDaemons,
		onRetry: () => {},
	},
};

export default meta;
type Story = StoryObj<typeof OrganizationProvisionerKeysPageView>;

export const Example: Story = {};

export const Paywalled: Story = {
	args: {
		showPaywall: true,
	},
};

export const NoProvisionerKeys: Story = {
	args: {
		provisionerKeyDaemons: [],
	},
};

export const ErrorLoadingProvisionerKeys: Story = {
	args: {
		error: "Failed to load provisioner keys",
	},
};
