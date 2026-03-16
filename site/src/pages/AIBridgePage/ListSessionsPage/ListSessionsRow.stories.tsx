import { MockSession } from "testHelpers/entities";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { Table, TableBody } from "components/Table/Table";
import { ListSessionsRow } from "./ListSessionsRow";

const meta: Meta<typeof ListSessionsRow> = {
	title: "pages/ListSessionsPage/ListSessionsRow",
	component: ListSessionsRow,
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

type Story = StoryObj<typeof ListSessionsRow>;

export const Session: Story = {
	args: {
		session: MockSession,
	},
};
