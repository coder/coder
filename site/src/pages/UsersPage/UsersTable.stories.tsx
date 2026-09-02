import type { Meta, StoryObj } from "@storybook/react-vite";
import type { FC } from "react";
import { useParams } from "react-router";
import { expect, fn, userEvent, within } from "storybook/test";
import { reactRouterParameters } from "storybook-addon-remix-react-router";
import {
	MockAuditorRole,
	MockGroup,
	MockMemberRole,
	MockTemplateAdminRole,
	MockUserAdminRole,
	MockUserMember,
	MockUserOwner,
} from "#/testHelpers/entities";
import { UsersTable } from "./UsersTable";

const mockGroupsByUserId = new Map([
	[MockUserOwner.id, [MockGroup]],
	[MockUserMember.id, [MockGroup]],
]);

const meta: Meta<typeof UsersTable> = {
	title: "pages/UsersPage/UsersTable",
	component: UsersTable,
	args: {
		me: MockUserOwner.id,
		onAction: fn(),
	},
};

export default meta;
type Story = StoryObj<typeof UsersTable>;

export const Example: Story = {
	args: {
		users: [MockUserOwner, MockUserMember],
		canEditUsers: false,
		groupsByUserId: mockGroupsByUserId,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(canvas.getByRole("table", { name: "Users" })).toBeVisible();
		await expect(canvas.getByText(MockUserOwner.username)).toBeVisible();
		await expect(
			canvas.queryByRole("button", { name: /open menu/i }),
		).not.toBeInTheDocument();
		await expect(
			canvas.queryByRole("link", { name: MockUserOwner.username }),
		).not.toBeInTheDocument();
	},
};

export const Editable: Story = {
	args: {
		users: [
			MockUserOwner,
			MockUserMember,
			{
				...MockUserOwner,
				id: "john-doe",
				username: "John Doe",
				email: "john.doe@coder.com",
				roles: [
					MockUserAdminRole,
					MockTemplateAdminRole,
					MockMemberRole,
					MockAuditorRole,
				],
				status: "dormant",
			},
			{
				...MockUserOwner,
				id: "roger-moore",
				username: "Roger Moore",
				email: "roger.moore@coder.com",
				roles: [],
				status: "suspended",
			},
			{
				...MockUserOwner,
				id: "oidc-user",
				username: "OIDC User",
				email: "oidc.user@coder.com",
				roles: [],
				status: "active",
				login_type: "oidc",
			},
		],
		canEditUsers: true,
		canViewActivity: true,
		groupsByUserId: mockGroupsByUserId,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const ownerRow = canvas.getByRole("row", { name: MockUserOwner.email });
		await userEvent.click(
			within(ownerRow).getByRole("button", { name: /open menu/i }),
		);
		const menu = within(document.body);
		await menu.findByRole("menuitem", { name: "Edit" });
		await menu.findByRole("menuitem", { name: "Edit roles" });
	},
};

const EditUserRoute: FC = () => {
	const { user } = useParams();
	return <div>Editing {user}</div>;
};

const editUserRouting = {
	reactRouter: reactRouterParameters({
		location: { path: "/deployment/users" },
		routing: [
			{ path: "/deployment/users", useStoryElement: true },
			{ path: "/deployment/users/:user", Component: EditUserRoute },
		],
	}),
};

export const OpensEditOnRowClick: Story = {
	args: { ...Editable.args },
	parameters: editUserRouting,
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const ownerRow = canvas.getByRole("row", { name: MockUserOwner.email });
		await userEvent.click(ownerRow);
		await expect(
			await canvas.findByText(`Editing ${MockUserOwner.username}`),
		).toBeVisible();
	},
};

export const NestedControlsDoNotOpenEdit: Story = {
	args: { ...Editable.args },
	parameters: editUserRouting,
	play: async ({ canvasElement, step }) => {
		const canvas = within(canvasElement);
		const body = within(document.body);
		const ownerRow = canvas.getByRole("row", { name: MockUserOwner.email });

		await step(
			"clicking the groups popover keeps the user on the table",
			async () => {
				await userEvent.click(
					within(ownerRow).getByRole("button", { name: /view 1 group/i }),
				);
				const groupsPopover = await body.findByRole("dialog");
				const groupName = await within(groupsPopover).findByText(
					MockGroup.display_name,
				);
				await userEvent.click(groupName);
				await expect(groupName).toBeInTheDocument();
				await userEvent.keyboard("{Escape}");
			},
		);

		await step(
			"opening the actions menu by keyboard keeps the user on the table",
			async () => {
				within(ownerRow)
					.getByRole("button", { name: /open menu/i })
					.focus();
				await userEvent.keyboard("{Enter}");
				await expect(
					await body.findByRole("menuitem", { name: "Edit" }),
				).toBeInTheDocument();
				await userEvent.keyboard("{Escape}");
			},
		);

		await expect(
			canvas.queryByText(`Editing ${MockUserOwner.username}`),
		).not.toBeInTheDocument();
		await expect(
			await canvas.findByRole("table", { name: "Users" }),
		).toBeVisible();
	},
};

export const Empty: Story = {
	args: {
		users: [],
	},
};

export const Loading: Story = {
	args: {
		users: [],
		isLoading: true,
	},
};
