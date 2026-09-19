import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn } from "storybook/test";
import {
	Table,
	TableBody,
	TableHead,
	TableHeader,
	TableRow,
} from "#/components/Table/Table";
import { SearchEmptyState, TableSearchEmpty } from "./SearchEmptyState";

const meta: Meta<typeof SearchEmptyState> = {
	title: "components/SearchEmptyState",
	component: SearchEmptyState,
	args: {
		message: "No users match your search",
	},
};

export default meta;
type Story = StoryObj<typeof SearchEmptyState>;

export const Default: Story = {};

export const WithResetFilters: Story = {
	args: {
		onClearFilters: fn(),
	},
};

export const CustomDescription: Story = {
	args: {
		message: "No groups match your search",
		description: "Try a different search term.",
		onClearFilters: fn(),
	},
};

export const InsideTable: Story = {
	render: (args) => (
		<Table aria-label="Users">
			<TableHeader>
				<TableRow>
					<TableHead>User</TableHead>
					<TableHead>Roles</TableHead>
					<TableHead>Groups</TableHead>
					<TableHead>Status</TableHead>
				</TableRow>
			</TableHeader>
			<TableBody>
				<TableSearchEmpty {...args} />
			</TableBody>
		</Table>
	),
	args: {
		onClearFilters: fn(),
	},
};
