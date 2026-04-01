import {
	MockOrganization,
	MockOrganization2,
	MockUserOwner,
} from "testHelpers/entities";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { action } from "storybook/actions";
import { userEvent, within } from "storybook/test";
import { OrganizationAutocomplete } from "./OrganizationAutocomplete";

const meta: Meta<typeof OrganizationAutocomplete> = {
	title: "components/OrganizationAutocomplete",
	component: OrganizationAutocomplete,
	args: {
		onChange: action("Selected organization"),
	},
};

export default meta;
type Story = StoryObj<typeof OrganizationAutocomplete>;

export const ManyOrgs: Story = {
	parameters: {
		showOrganizations: true,
		user: MockUserOwner,
		features: ["multiple_organizations"],
		permissions: { viewDeploymentConfig: true },
		queries: [
			{
				key: ["organizations"],
				data: [MockOrganization, MockOrganization2],
			},
		],
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const button = canvas.getByRole("button");
		await userEvent.click(button);
	},
};

export const OneOrg: Story = {
	parameters: {
		showOrganizations: true,
		user: MockUserOwner,
		features: ["multiple_organizations"],
		permissions: { viewDeploymentConfig: true },
		queries: [
			{
				key: ["organizations"],
				data: [MockOrganization],
			},
		],
	},
};
