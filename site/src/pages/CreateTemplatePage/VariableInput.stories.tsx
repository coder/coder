import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fn, within } from "storybook/test";
import type { TemplateVersionVariable } from "#/api/typesGenerated";
import { VariableInput } from "./VariableInput";

const baseVariable: TemplateVersionVariable = {
	name: "region",
	description: "The deployment region.",
	type: "string",
	value: "",
	default_value: "us-east-1",
	required: false,
	sensitive: false,
};

const meta: Meta<typeof VariableInput> = {
	title: "pages/CreateTemplatePage/VariableInput",
	component: VariableInput,
	args: {
		onChange: fn(),
		variable: baseVariable,
	},
};

export default meta;
type Story = StoryObj<typeof VariableInput>;

export const PlainDescription: Story = {};

export const MarkdownDescription: Story = {
	args: {
		variable: {
			...baseVariable,
			name: "bingbong",
			description:
				"I am trying [markdown](https://google.com) with **bold** and `code`.",
		},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		// The Markdown link renders as an anchor pointing at the URL.
		const link = canvas.getByRole("link", { name: /markdown/i });
		await expect(link).toHaveAttribute("href", "https://google.com");
		// Inline formatting renders rather than showing raw Markdown syntax.
		await expect(canvas.queryByText(/\[markdown\]/)).not.toBeInTheDocument();
	},
};

export const Optional: Story = {
	args: {
		variable: {
			...baseVariable,
			name: "docker_socket",
			description: "(Optional) Docker socket URI",
		},
	},
};
