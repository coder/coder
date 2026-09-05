import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, userEvent, waitFor, within } from "storybook/test";
import { workspacesKey } from "#/api/queries/workspaces";
import {
	MockTemplate,
	MockTemplateVersion,
	MockWorkspace,
} from "#/testHelpers/entities";
import { withDashboardProvider } from "#/testHelpers/storybook";
import { TemplatePageHeader } from "./TemplatePageHeader";

const meta: Meta<typeof TemplatePageHeader> = {
	title: "pages/TemplatePage/TemplatePageHeader",
	component: TemplatePageHeader,
	decorators: [withDashboardProvider],
	parameters: {
		queries: [
			{
				key: workspacesKey({
					q: `organization:${MockTemplate.organization_name} template:${MockTemplate.name}`,
					limit: 1,
				}),
				data: {
					workspaces: [],
					count: 0,
				},
			},
		],
	},
	args: {
		template: MockTemplate,
		activeVersion: MockTemplateVersion,
		permissions: {
			canUpdateTemplate: true,
		},
		workspacePermissions: {
			createWorkspaceForUserID: true,
		},
	},
};

export default meta;
type Story = StoryObj<typeof TemplatePageHeader>;

export const Example: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(canvas.getByRole("button", { name: "Open menu" }));

		const menu = within(document.body);
		await expect(
			menu.getByRole("menuitem", { name: "Settings" }),
		).toHaveAttribute("href", `/templates/${MockTemplate.name}/settings`);
		await expect(
			menu.getByRole("menuitem", { name: "Edit files" }),
		).toHaveAttribute(
			"href",
			`/templates/${MockTemplate.name}/versions/${MockTemplateVersion.name}/edit`,
		);
		await expect(
			menu.getByRole("menuitem", { name: "Duplicate…" }),
		).toHaveAttribute("href", `/templates/new?fromTemplate=${MockTemplate.id}`);

		await userEvent.keyboard("{Escape}");
		await waitFor(() => {
			expect(
				menu.queryByRole("menuitem", { name: "Settings" }),
			).not.toBeInTheDocument();
		});
	},
};

export const CanNotUpdate: Story = {
	args: {
		permissions: {
			canUpdateTemplate: false,
		},
	},
};

export const HasWorkspaces: Story = {
	parameters: {
		queries: [
			{
				key: workspacesKey({
					q: `organization:${MockTemplate.organization_name} template:${MockTemplate.name}`,
					limit: 1,
				}),
				data: {
					workspaces: [MockWorkspace],
					count: 1,
				},
			},
		],
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const templateMenu = canvas.getByLabelText("Open menu");
		await userEvent.click(templateMenu);
		const deleteOption = within(document.body).getByText("Delete…");
		await userEvent.click(deleteOption);
	},
};

export const CannotCreateWorkspace: Story = {
	args: {
		workspacePermissions: {
			createWorkspaceForUserID: false,
		},
	},
};

export const Deprecated: Story = {
	args: {
		template: {
			...MockTemplate,
			deprecated: true,
			deprecation_message:
				"This template is not going to be used anymore. [See details](#details).",
		},
	},
};
