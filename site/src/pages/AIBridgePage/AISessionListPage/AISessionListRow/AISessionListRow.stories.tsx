import { MockSession } from "testHelpers/entities";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { Table, TableBody } from "components/Table/Table";
import { AISessionListRow } from "./AISessionListRow";

const meta: Meta<typeof AISessionListRow> = {
	title: "pages/AIBridgePage/AISessionListRow",
	component: AISessionListRow,
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
type Story = StoryObj<typeof AISessionListRow>;

export const Session: Story = {
	args: {
		session: MockSession,
	},
};
