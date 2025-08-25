import { chromaticWithTablet } from "testHelpers/chromatic";
import {
	MockUserMember,
	MockUserOwner,
	MockWorkspace,
	MockWorkspaceAppStatus,
} from "testHelpers/entities";
import { withDashboardProvider } from "testHelpers/storybook";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { userEvent, within } from "storybook/test";
import { NavbarView } from "./NavbarView";

const tasksFilter = {
	username: MockUserOwner.username,
};

const meta: Meta<typeof NavbarView> = {
	title: "modules/dashboard/NavbarView",
	parameters: {
		chromatic: chromaticWithTablet,
		layout: "fullscreen",
		queries: [
			{
				key: ["tasks", tasksFilter],
				data: [],
			},
		],
	},
	component: NavbarView,
	args: {
		user: MockUserOwner,
		canViewAuditLog: true,
		canViewDeployment: true,
		canViewHealth: true,
		canViewOrganizations: true,
	},
	decorators: [withDashboardProvider],
};

export default meta;
type Story = StoryObj<typeof NavbarView>;

export const ForAdmin: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(
			canvas.getByRole("button", { name: "Admin settings" }),
		);
	},
};

export const ForAuditor: Story = {
	args: {
		user: MockUserMember,
		canViewAuditLog: true,
		canViewDeployment: false,
		canViewHealth: false,
		canViewOrganizations: false,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(
			canvas.getByRole("button", { name: "Admin settings" }),
		);
	},
};

export const ForOrgAdmin: Story = {
	args: {
		user: MockUserMember,
		canViewAuditLog: true,
		canViewDeployment: false,
		canViewHealth: false,
		canViewOrganizations: true,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(
			canvas.getByRole("button", { name: "Admin settings" }),
		);
	},
};

export const ForMember: Story = {
	args: {
		user: MockUserMember,
		canViewAuditLog: false,
		canViewDeployment: false,
		canViewHealth: false,
		canViewOrganizations: false,
	},
};

export const CustomLogo: Story = {
	args: {
		logo_url: "/icon/github.svg",
	},
};

export const IdleTasks: Story = {
	parameters: {
		queries: [
			{
				key: ["tasks", tasksFilter],
				data: [
					{
						prompt: "Task 1",
						workspace: {
							...MockWorkspace,
							latest_app_status: {
								...MockWorkspaceAppStatus,
								state: "idle",
							},
						},
					},
					{
						prompt: "Task 2",
						workspace: MockWorkspace,
					},
					{
						prompt: "Task 3",
						workspace: {
							...MockWorkspace,
							latest_app_status: MockWorkspaceAppStatus,
						},
					},
				],
			},
		],
	},
};
