import { action } from "@storybook/addon-actions";
import type { Meta, StoryObj } from "@storybook/react";
import { userEvent, within } from "@storybook/test";
import { organizationsKey } from "api/queries/organizations";
import type { Organization } from "api/typesGenerated";
import {
	MockOrganization,
	MockOrganization2,
	mockApiError,
} from "testHelpers/entities";
import { CreateUserForm } from "./CreateUserForm";

const meta: Meta<typeof CreateUserForm> = {
	title: "pages/CreateUserPage",
	component: CreateUserForm,
	args: {
		onCancel: action("cancel"),
		onSubmit: action("submit"),
		isLoading: false,
	},
};

export default meta;
type Story = StoryObj<typeof CreateUserForm>;

export const Ready: Story = {};

const permissionCheckQuery = (organizations: Organization[]) => {
	return {
		key: [
			"authorization",
			{
				checks: Object.fromEntries(
					organizations.map((org) => [
						org.id,
						{
							action: "create",
							object: {
								resource_type: "organization_member",
								organization_id: org.id,
							},
						},
					]),
				),
			},
		],
		data: Object.fromEntries(organizations.map((org) => [org.id, true])),
	};
};

export const WithOrganizations: Story = {
	parameters: {
		queries: [
			{
				key: organizationsKey,
				data: [MockOrganization, MockOrganization2],
			},
			permissionCheckQuery([MockOrganization, MockOrganization2]),
		],
	},
	args: {
		showOrganizations: true,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(canvas.getByLabelText("Organization *"));
	},
};

export const FormError: Story = {
	args: {
		error: mockApiError({
			validations: [{ field: "username", detail: "Username taken" }],
		}),
	},
};

export const GeneralError: Story = {
	args: {
		error: mockApiError({
			message: "User already exists",
		}),
	},
};

export const Loading: Story = {
	args: {
		isLoading: true,
	},
};
