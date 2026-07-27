import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fn, userEvent, within } from "storybook/test";
import { MockTemplateVersionParameter1 } from "#/testHelpers/entities";
import { UpdateBuildParametersDialog } from "./UpdateBuildParametersDialog";

const meta = {
	title: "modules/workspaces/UpdateBuildParametersDialog",
	component: UpdateBuildParametersDialog,
	args: {
		open: true,
		onClose: fn(),
		onUpdate: fn(),
		missedParameters: [MockTemplateVersionParameter1],
	},
} satisfies Meta<typeof UpdateBuildParametersDialog>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Example: Story = {};

export const SubmitsEnteredValues: Story = {
	play: async ({ args }) => {
		const body = within(document.body);
		const input = await body.findByRole("textbox");
		await userEvent.clear(input);
		await userEvent.type(input, "us-west");
		await userEvent.click(
			body.getByRole("button", { name: "Update parameters" }),
		);
		await expect(args.onUpdate).toHaveBeenCalledWith([
			{ name: "first_parameter", value: "us-west" },
		]);
	},
};
