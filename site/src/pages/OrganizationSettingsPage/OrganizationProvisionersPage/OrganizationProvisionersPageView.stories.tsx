import type { Meta, StoryObj } from "@storybook/react";
import {
	MockBuildInfo,
	MockProvisioner,
	MockProvisionerWithTags,
	MockUserProvisioner,
	mockApiError,
} from "testHelpers/entities";
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
		],
		filter: {
			ids: "",
		},
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
};

export const FilterByID: Story = {
	args: {
		provisioners: [MockProvisioner],
		filter: {
			ids: MockProvisioner.id,
		},
	},
};
