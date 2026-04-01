import type { Meta, StoryObj } from "@storybook/react-vite";
import { action } from "storybook/actions";
import { expect, fn, userEvent, waitFor, within } from "storybook/test";
import {
	MockOrganization,
	MockOrganization2,
	MockUserOwner,
} from "#/testHelpers/entities";
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

export const PreselectedOrg: Story = {
	args: {
		organizationId: MockOrganization2.id,
		onChange: fn<(value: unknown) => void>(),
	},
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
	play: async ({ canvasElement, args }) => {
		const canvas = within(canvasElement);
		const button = canvas.getByRole("button");
		await waitFor(() =>
			expect(button).toHaveTextContent(MockOrganization2.display_name),
		);
		const onChangeSpy = args.onChange as ReturnType<
			typeof fn<(value: unknown) => void>
		>;
		expect(onChangeSpy).not.toHaveBeenCalled();
	},
};

export const PreselectedOrgNotFound: Story = {
	args: {
		organizationId: "nonexistent-id",
	},
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
		expect(button).toHaveTextContent("Select an organization");
	},
};

export const OneOrgWithControlledId: Story = {
	args: {
		organizationId: MockOrganization.id,
		onChange: fn<(value: unknown) => void>(),
	},
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
	play: async ({ canvasElement, args }) => {
		const canvas = within(canvasElement);
		const button = canvas.getByRole("button");
		await waitFor(() =>
			expect(button).toHaveTextContent(MockOrganization.display_name),
		);
		const onChangeSpy = args.onChange as ReturnType<
			typeof fn<(value: unknown) => void>
		>;
		expect(onChangeSpy).not.toHaveBeenCalled();
	},
};
