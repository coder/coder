import type { Meta, StoryObj } from "@storybook/react-vite";
import { PlusIcon } from "lucide-react";
import { fn } from "storybook/test";
import { Button } from "#/components/Button/Button";
import {
	Table,
	TableBody,
	TableHead,
	TableHeader,
	TableRow,
} from "#/components/Table/Table";
import { TableEmptyState } from "./TableEmptyState";

const meta: Meta<typeof TableEmptyState> = {
	title: "components/TableEmptyState",
	component: TableEmptyState,
	decorators: [
		(Story) => (
			<Table aria-label="Example">
				<TableHeader>
					<TableRow>
						<TableHead>Name</TableHead>
						<TableHead>Status</TableHead>
					</TableRow>
				</TableHeader>
				<TableBody>
					<Story />
				</TableBody>
			</Table>
		),
	],
};

export default meta;
type Story = StoryObj<typeof TableEmptyState>;

export const Search: Story = {
	args: {
		variant: "search",
		message: "No users match your search",
		onResetFilters: fn(),
	},
};

export const Docs: Story = {
	args: {
		variant: "docs",
		message: "No group mappings configured",
		description:
			"Automatically assign users to groups based on their identity provider claims.",
		docsHref: "https://docs.example.com/idp-sync",
		docsLabel: "How to set up IdP group sync",
	},
};

export const Create: Story = {
	args: {
		variant: "create",
		message: "No MCP servers configured",
		description: "Add a server to give agents access to external tools.",
		cta: (
			<Button>
				<PlusIcon />
				Add server
			</Button>
		),
	},
};

export const Upgrade: Story = {
	args: {
		variant: "upgrade",
		message: "No custom roles yet",
		description: "Upgrade to a premium license to create custom roles.",
	},
};

export const Default: Story = {
	args: {
		variant: "default",
		message: "No provisioner jobs found",
	},
};
