import type { Meta, StoryObj } from "@storybook/react-vite";
import type { FC } from "react";
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

// Renders the tooltip inside a row whose own onClick navigates on click. The
// tooltip's trigger must stop propagation so the popover can open instead of
// the parent row swallowing the click. Regression coverage for the
// `useClickableTableRow` usage on the workspaces list.
const onRowClick = fn();

const ClickableRowDecorator = (Story: FC) => {
	const clickableProps = useClickableTableRow({ onClick: onRowClick });
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
};

export const InsideClickableRow: Story = {
	decorators: [ClickableRowDecorator],
	beforeEach: () => {
		onRowClick.mockClear();
	},
	play: async ({ canvasElement, step }) => {
		const body = within(canvasElement.ownerDocument.body);

		await step("clicking the trigger opens the popover", async () => {
			await userEvent.click(body.getByRole("button", { name: "More info" }));
			await waitFor(() =>
				expect(screen.getByRole("dialog")).toHaveTextContent(
					MockTemplateVersion.message,
				),
			);
		});

		await step("clicking the trigger does not navigate the row", async () => {
			expect(onRowClick).not.toHaveBeenCalled();
		});
	},
};
