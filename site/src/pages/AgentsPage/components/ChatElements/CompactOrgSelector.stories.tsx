import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn } from "storybook/test";
import type { Organization } from "#/api/typesGenerated";
import { CompactOrgSelector } from "./CompactOrgSelector";

const mockOrgs: Organization[] = [
	{
		id: "org-coder",
		name: "coder",
		display_name: "Coder",
		icon: "/icon/coder.svg",
		description: "Main engineering organization",
		created_at: "2024-01-01T00:00:00Z",
		updated_at: "2024-06-01T00:00:00Z",
		is_default: true,
	},
	{
		id: "org-acme",
		name: "acme-corp",
		display_name: "Acme Corp",
		icon: "",
		description: "Acme Corporation",
		created_at: "2024-02-01T00:00:00Z",
		updated_at: "2024-06-01T00:00:00Z",
		is_default: false,
	},
	{
		id: "org-globex",
		name: "globex",
		display_name: "Globex Inc",
		icon: "",
		description: "Globex Incorporated",
		created_at: "2024-03-01T00:00:00Z",
		updated_at: "2024-06-01T00:00:00Z",
		is_default: false,
	},
];

const meta: Meta<typeof CompactOrgSelector> = {
	title: "pages/AgentsPage/ChatElements/CompactOrgSelector",
	component: CompactOrgSelector,
	decorators: [
		(Story) => (
			<div className="w-72 rounded-lg border border-solid border-border-default bg-surface-primary p-4">
				<Story />
			</div>
		),
	],
	args: {
		options: mockOrgs,
		value: mockOrgs[0],
		onChange: fn(),
	},
};

export default meta;
type Story = StoryObj<typeof CompactOrgSelector>;

export const Default: Story = {};

export const Disabled: Story = {
	args: {
		disabled: true,
		value: mockOrgs[0],
	},
};

export const NoSelection: Story = {
	args: {
		value: null,
	},
};

export const SingleOption: Story = {
	args: {
		options: [mockOrgs[0]],
		value: mockOrgs[0],
	},
};
