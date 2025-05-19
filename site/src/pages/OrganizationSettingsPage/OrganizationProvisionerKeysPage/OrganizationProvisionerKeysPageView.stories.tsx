import type { Meta, StoryObj } from "@storybook/react";
import {
	type ProvisionerKeyDaemons,
	ProvisionerKeyIDBuiltIn,
	ProvisionerKeyIDPSK,
	ProvisionerKeyIDUserAuth,
} from "api/typesGenerated";
import { MockProvisioner, MockProvisionerKey } from "testHelpers/entities";
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
};

export const Empty: Story = {
	...Default,
	args: {
		provisionerKeyDaemons: [],
	},
};

export const ErrorLoadingProvisionerKeys: Story = {
	...Default,
	args: {
		provisionerKeyDaemons: [],
		error: "Failed to load provisioner keys",
	},
};
