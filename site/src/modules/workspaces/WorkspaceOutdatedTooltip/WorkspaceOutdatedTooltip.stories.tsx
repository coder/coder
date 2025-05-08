import type { Meta, StoryObj } from "@storybook/react";
import { expect, userEvent, waitFor, within } from "@storybook/test";
import {
	MockTemplate,
	MockTemplateVersion,
	MockWorkspace,
} from "testHelpers/entities";
import { withDashboardProvider } from "testHelpers/storybook";
import { WorkspaceOutdatedTooltip } from "./WorkspaceOutdatedTooltip";

const meta: Meta<typeof WorkspaceOutdatedTooltip> = {
	title: "modules/workspaces/WorkspaceOutdatedTooltip",
	component: WorkspaceOutdatedTooltip,
	decorators: [withDashboardProvider],
	parameters: {
		queries: [
			{
				key: ["templateVersion", MockTemplateVersion.id],
				data: MockTemplateVersion,
			},
		],
	},
	args: {
		workspace: {
			...MockWorkspace,
			template_name: MockTemplate.display_name,
			template_active_version_id: MockTemplateVersion.id,
		},
	},
};

export default meta;
type Story = StoryObj<typeof WorkspaceOutdatedTooltip>;

const Example: Story = {
	play: async ({ canvasElement, step }) => {
		const body = within(canvasElement.ownerDocument.body);

		await step("activate hover trigger", async () => {
			await userEvent.hover(body.getByRole("button"));
			await waitFor(() =>
				expect(body.getByText(MockTemplateVersion.message)).toBeInTheDocument(),
			);
		});
	},
};

export { Example as WorkspaceOutdatedTooltip };
