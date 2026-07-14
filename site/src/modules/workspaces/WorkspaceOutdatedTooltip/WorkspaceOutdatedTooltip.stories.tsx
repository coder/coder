import type { Meta, StoryObj } from "@storybook/react-vite";
import type { ComponentProps } from "react";
import { expect, fn, screen, userEvent, waitFor, within } from "storybook/test";
import {
	Table,
	TableBody,
	TableCell,
	TableRow,
} from "#/components/Table/Table";
import { useClickableTableRow } from "#/hooks/useClickableTableRow";
import {
	MockTemplate,
	MockTemplateVersion,
	MockWorkspace,
} from "#/testHelpers/entities";
import { withDashboardProvider } from "#/testHelpers/storybook";
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
			await userEvent.click(body.getByRole("button"));
			await waitFor(() =>
				expect(screen.getByRole("dialog")).toHaveTextContent(
					MockTemplateVersion.message,
				),
			);
		});
	},
};

export { Example as WorkspaceOutdatedTooltip };

// Regression coverage for the `useClickableTableRow` usage on the workspaces
// list. The trigger must stop click + keyboard propagation so the popover
// opens instead of the parent row's onClick swallowing the activation and
// navigating away.
type ClickableRowArgs = ComponentProps<typeof WorkspaceOutdatedTooltip> & {
	onRowClick: () => void;
};

export const InsideClickableRow: StoryObj<ClickableRowArgs> = {
	args: {
		onRowClick: fn(),
	},
	decorators: [
		(Story, { args }) => {
			const clickableProps = useClickableTableRow({
				onClick: args.onRowClick,
			});
			return (
				<Table>
					<TableBody>
						<TableRow {...clickableProps}>
							<TableCell>
								<Story />
							</TableCell>
						</TableRow>
					</TableBody>
				</Table>
			);
		},
	],
	play: async ({ args, canvasElement, step }) => {
		const body = within(canvasElement.ownerDocument.body);

		await step("mouse click opens the popover", async () => {
			await userEvent.click(body.getByRole("button", { name: "More info" }));
			await waitFor(() =>
				expect(screen.getByRole("dialog")).toHaveTextContent(
					MockTemplateVersion.message,
				),
			);
			await userEvent.keyboard("{Escape}");
		});

		await step("keyboard activation via Space opens the popover", async () => {
			body.getByRole("button", { name: "More info" }).focus();
			await userEvent.keyboard(" ");
			await waitFor(() =>
				expect(screen.getByRole("dialog")).toHaveTextContent(
					MockTemplateVersion.message,
				),
			);
		});

		await step("the row's onClick was never called", async () => {
			expect(args.onRowClick).not.toHaveBeenCalled();
		});
	},
};
