import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn, screen, userEvent, within } from "storybook/test";
import type { Organization } from "#/api/typesGenerated";
import { MockOrganization } from "#/testHelpers/entities";
import { CompactOrgSelector } from "./CompactOrgSelector";

const mockOrgs: Organization[] = [
	{
		...MockOrganization,
		id: "org-coder",
		name: "coder",
		display_name: "Coder",
		icon: "/icon/coder.svg",
		description: "Main engineering organization",
		is_default: true,
	},
	{
		...MockOrganization,
		id: "org-acme",
		name: "acme-corp",
		display_name: "Acme Corp",
		icon: "",
		description: "Acme Corporation",
		is_default: false,
	},
	{
		...MockOrganization,
		id: "org-globex",
		name: "globex",
		display_name: "Globex Inc",
		icon: "",
		description: "Globex Incorporated",
		is_default: false,
	},
];

const meta: Meta<typeof CompactOrgSelector> = {
	title: "pages/AgentsPage/ChatElements/CompactOrgSelector",
	component: CompactOrgSelector,
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

export const Open: Story = {
	play: async ({ canvasElement }) => {
		await userEvent.click(
			within(canvasElement).getByRole("button", {
				name: "Organization: Coder",
			}),
		);
	},
};

export const Filtered: Story = {
	play: async ({ canvasElement }) => {
		await userEvent.click(
			within(canvasElement).getByRole("button", {
				name: "Organization: Coder",
			}),
		);
		await userEvent.type(
			screen.getByRole("combobox", { name: "Search organizations" }),
			"Acme",
		);
	},
};

export const EmptySearch: Story = {
	play: async ({ canvasElement }) => {
		await userEvent.click(
			within(canvasElement).getByRole("button", {
				name: "Organization: Coder",
			}),
		);
		await userEvent.type(
			screen.getByRole("combobox", { name: "Search organizations" }),
			"missing",
		);
	},
};
