import { chromaticWithTablet } from "testHelpers/chromatic";
import { MockTasks, MockUserMember, MockUserOwner } from "testHelpers/entities";
import { withDashboardProvider } from "testHelpers/storybook";
import type { Meta, StoryObj } from "@storybook/react-vite";
import type { TasksFilter } from "api/typesGenerated";
import { userEvent, within } from "storybook/test";
import { NavbarView } from "./NavbarView";

const tasksFilter: TasksFilter = {
	owner: MockUserOwner.username,
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
		supportLinks: [],
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
				data: MockTasks,
			},
		],
	},
};

export const SupportLinks: Story = {
	args: {
		user: MockUserMember,
		canViewAuditLog: false,
		canViewDeployment: false,
		canViewHealth: false,
		canViewOrganizations: false,
		supportLinks: [
			{
				name: "This is a bug",
				icon: "bug",
				target: "#",
			},
			{
				name: "This is a star",
				icon: "star",
				target: "#",
				location: "navbar",
			},
			{
				name: "This is a chat",
				icon: "chat",
				target: "#",
				location: "navbar",
			},
			{
				name: "No icon here",
				icon: "",
				target: "#",
				location: "navbar",
			},
			{
				name: "No icon here too",
				icon: "",
				target: "#",
			},
		],
	},
};

export const DefaultSupportLinks: Story = {
	args: {
		user: MockUserMember,
		canViewAuditLog: false,
		canViewDeployment: false,
		canViewHealth: false,
		canViewOrganizations: false,
		supportLinks: [
			{ icon: "docs", name: "Documentation", target: "" },
			{ icon: "bug", name: "Report a bug", target: "" },
			{
				icon: "chat",
				name: "Join the Coder Discord",
				target: "",
				location: "navbar",
			},
			{ icon: "star", name: "Star the Repo", target: "" },
		],
	},
};
