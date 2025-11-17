import { MockInterception } from "testHelpers/entities";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { Table, TableBody } from "components/Table/Table";
import { RequestLogsRow } from "./RequestLogsRow";

const meta: Meta<typeof RequestLogsRow> = {
	title: "pages/AIBridgePage/RequestLogsRow",
	component: RequestLogsRow,
	decorators: [
		(Story) => (
			<Table>
				<TableBody>
					<Story />
				</TableBody>
			</Table>
		),
	],
};

export default meta;
type Story = StoryObj<typeof RequestLogsRow>;

export const Close: Story = {
	args: {
		interception: MockInterception,
	},
};
