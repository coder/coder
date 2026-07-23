import type { Meta, StoryObj } from "@storybook/react-vite";
import { action } from "storybook/actions";
import { MockPreviewParameter, MockTemplate } from "#/testHelpers/entities";
import { TemplateEmbedPageView } from "./TemplateEmbedPageView";

const meta: Meta<typeof TemplateEmbedPageView> = {
	title: "pages/TemplatePage/TemplateEmbedPageView",
	component: TemplateEmbedPageView,
	args: {
		template: MockTemplate,
		parameters: [
			{
				...MockPreviewParameter,
				name: "eevee",
				display_name: "Favorite Eevee",
				form_type: "input",
				value: { value: "Glaceon", valid: true },
				default_value: { value: "Jolteon", valid: true },
				order: 1,
			},
			{
				...MockPreviewParameter,
				name: "commander",
				display_name: "Commander",
				form_type: "input",
				value: { value: "Lyra Dawnbringer", valid: true },
				default_value: { value: "", valid: true },
				order: 0,
			},
		],
		diagnostics: [],
		sendMessage: action("sendMessage"),
		isLoading: false,
	},
};

export default meta;
type Story = StoryObj<typeof TemplateEmbedPageView>;

export const Example: Story = {};

export const Loading: Story = {
	args: {
		isLoading: true,
	},
};

export const WithError: Story = {
	args: {
		error: new Error("failed to connect"),
		parameters: [],
	},
};

export const Validation: Story = {
	args: {
		diagnostics: [
			{
				severity: "warning",
				summary: "Invalid wibble",
				detail: "Needs more wobble",
				extra: {
					code: "wooble",
				},
			},
		],
	},
};
