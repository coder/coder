import type { Meta, StoryObj } from "@storybook/react-vite";
import { action } from "storybook/actions";
import { expect, fn, screen, userEvent, waitFor, within } from "storybook/test";
import type { Organization } from "#/api/typesGenerated";
import {
	MockOrganization,
	MockOrganization2,
	MockUserOwner,
} from "#/testHelpers/entities";
import { OrganizationAutocomplete } from "./OrganizationAutocomplete";

type OnChangeFn = (org: Organization | null) => void;

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
		onChange: fn<OnChangeFn>(),
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
		const onChangeSpy = args.onChange as ReturnType<typeof fn<OnChangeFn>>;
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
		// Open the dropdown to verify data has loaded.
		await userEvent.click(button);
		await waitFor(() =>
			expect(
				screen.getByText(MockOrganization.display_name),
			).toBeInTheDocument(),
		);
		// Close and verify the button still shows placeholder
		// (the org ID doesn't match any loaded option).
		await userEvent.keyboard("{Escape}");
		await waitFor(() =>
			expect(button).toHaveTextContent("Select an organization"),
		);
	},
};

export const PreselectedOrgUserSelects: Story = {
	args: {
		organizationId: MockOrganization2.id,
		onChange: fn<OnChangeFn>(),
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
		// Wait for the preselected org to appear.
		await waitFor(() =>
			expect(button).toHaveTextContent(MockOrganization2.display_name),
		);
		const onChangeSpy = args.onChange as ReturnType<typeof fn<OnChangeFn>>;
		onChangeSpy.mockClear();
		// Open dropdown and select a different org.
		await userEvent.click(button);
		await waitFor(() =>
			expect(
				screen.getByText(MockOrganization.display_name),
			).toBeInTheDocument(),
		);
		await userEvent.click(screen.getByText(MockOrganization.display_name));
		// Verify onChange was called with the new org.
		await waitFor(() =>
			expect(onChangeSpy).toHaveBeenCalledWith(
				expect.objectContaining({ id: MockOrganization.id }),
			),
		);
		// Button should still show the prop-controlled value since
		// the parent hasn't updated organizationId.
		await waitFor(() =>
			expect(button).toHaveTextContent(MockOrganization2.display_name),
		);
	},
};

export const OneOrgWithControlledId: Story = {
	args: {
		organizationId: MockOrganization.id,
		onChange: fn<OnChangeFn>(),
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
		const onChangeSpy = args.onChange as ReturnType<typeof fn<OnChangeFn>>;
		expect(onChangeSpy).not.toHaveBeenCalled();
	},
};
