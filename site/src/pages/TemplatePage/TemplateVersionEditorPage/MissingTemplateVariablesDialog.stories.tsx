import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fn, userEvent, within } from "storybook/test";
import { MockTemplateVersionVariable5 } from "#/testHelpers/entities";
import { MissingTemplateVariablesDialog } from "./MissingTemplateVariablesDialog";

const meta = {
	title:
		"pages/TemplatePage/TemplateVersionEditorPage/MissingTemplateVariablesDialog",
	component: MissingTemplateVariablesDialog,
	args: {
		open: true,
		onClose: fn(),
		onSubmit: fn(),
		missingVariables: [MockTemplateVersionVariable5],
	},
} satisfies Meta<typeof MissingTemplateVariablesDialog>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Example: Story = {};

export const SubmitsEnteredValues: Story = {
	play: async ({ args }) => {
		const body = within(document.body);
		const input = await body.findByRole("textbox");
		await userEvent.type(input, "production");
		await userEvent.click(body.getByRole("button", { name: "Submit" }));
		await expect(args.onSubmit).toHaveBeenCalledWith([
			{ name: "fifth_variable", value: "production" },
		]);
	},
};
