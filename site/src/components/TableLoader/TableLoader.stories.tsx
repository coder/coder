import type { Meta, StoryObj } from "@storybook/react-vite";
import { Table, TableBody } from "components/Table/Table";
import { TableLoader } from "./TableLoader";

const meta: Meta<typeof TableLoader> = {
	title: "components/TableLoader",
	component: TableLoader,
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
type Story = StoryObj<typeof TableLoader>;

export const Example: Story = {};
export { Example as TableLoader };
