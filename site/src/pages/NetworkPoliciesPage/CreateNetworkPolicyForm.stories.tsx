import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn, userEvent, within } from "storybook/test";
import { MockTemplate, mockApiError } from "#/testHelpers/entities";
import { MockNetworkPolicyTemplate } from "#/testHelpers/networkPolicies";
import { CreateNetworkPolicyForm } from "./CreateNetworkPolicyForm";

const meta = {
	title: "pages/NetworkPolicies/Create",
	component: CreateNetworkPolicyForm,
	args: {
		templates: [MockTemplate, MockNetworkPolicyTemplate],
		isPending: false,
		error: undefined,
		onSubmit: fn(),
	},
} satisfies Meta<typeof CreateNetworkPolicyForm>;
export default meta;
type Story = StoryObj<typeof meta>;
export const Default: Story = {};
export const NoTemplates: Story = { args: { templates: [] } };
export const Saving: Story = { args: { isPending: true } };
export const SaveError: Story = {
	args: {
		error: mockApiError({
			message: "The policy HCL is invalid.",
			validations: [
				{
					field: "hcl",
					detail:
						"Only the trusted coder_network_policy module may be configured.",
				},
			],
		}),
	},
	play: async ({ canvasElement }) => {
		await userEvent.click(
			await within(canvasElement).findByRole("textbox", { name: "Policy HCL" }),
		);
		await userEvent.tab();
	},
};
export const TemplateSelection: Story = {
	play: async ({ canvasElement }) => {
		await userEvent.click(
			within(canvasElement).getByRole("combobox", { name: "Templates" }),
		);
	},
};
export const KeyboardFocus: Story = {
	play: async ({ canvasElement }) => {
		(
			await within(canvasElement).findByRole("textbox", { name: "Policy HCL" })
		).focus();
	},
};
export const Mobile: Story = {
	globals: { viewport: { value: "iphone12", isRotated: false } },
};
