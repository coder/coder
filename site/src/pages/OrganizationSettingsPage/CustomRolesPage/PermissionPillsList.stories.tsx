import type { Meta, StoryObj } from "@storybook/react";
import { userEvent, within } from "@storybook/test";
import { MockRoleWithOrgPermissions } from "testHelpers/entities";
import { PermissionPillsList } from "./PermissionPillsList";

const meta: Meta<typeof PermissionPillsList> = {
	title: "pages/OrganizationCustomRolesPage/PermissionPillsList",
	component: PermissionPillsList,
	decorators: [
		(Story) => (
			<div style={{ width: "800px" }}>
				<Story />
			</div>
		),
	],
	parameters: {
		chromatic: {
			diffThreshold: 0.5,
		},
	},
};

export default meta;
type Story = StoryObj<typeof PermissionPillsList>;

export const Default: Story = {
	args: {
		permissions: MockRoleWithOrgPermissions.organization_permissions,
	},
};

export const SinglePermission: Story = {
	args: {
		permissions: [
			{
				negate: false,
				resource_type: "organization_member",
				action: "create",
			},
		],
	},
};

export const NoPermissions: Story = {
	args: {
		permissions: [],
	},
};

export const HoverOverflowPill: Story = {
	args: {
		permissions: MockRoleWithOrgPermissions.organization_permissions,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.hover(canvas.getByTestId("overflow-permissions-pill"));
	},
};
