import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fn, within } from "storybook/test";
import type { TemplateBuilderModulesResponse } from "#/api/typesGenerated";
import { ModuleSettingsStep } from "./ModuleSettingsStep";

const baseId = "docker";

const modulesData: TemplateBuilderModulesResponse = {
	modules: [
		{
			id: "jetbrains",
			display_name: "JetBrains Toolbox",
			description: "Add JetBrains IDE integrations to your Coder workspaces.",
			icon: "/icon/jetbrains.svg",
			category: "IDE",
			version: "1.0.0",
			compatible_os: ["linux"],
			conflicts_with: [],
			variables: [
				{
					name: "folder",
					type: "string",
					description: "The directory to open in the IDE.",
					required: true,
					sensitive: false,
				},
			],
		},
	],
};

const meta: Meta<typeof ModuleSettingsStep> = {
	title: "pages/TemplateBuilder/ModuleSettingsStep",
	component: ModuleSettingsStep,
	args: {
		baseId,
		selectedModuleIds: ["jetbrains"],
		moduleVariables: {},
		onChangeModuleVariables: fn(),
		onRemoveModule: fn(),
		registerModuleRef: fn(),
	},
	parameters: {
		queries: [
			{
				key: ["templateBuilder", "modules", baseId],
				data: modulesData,
			},
		],
	},
};

export default meta;
type Story = StoryObj<typeof ModuleSettingsStep>;

export const Default: Story = {};

// With showErrors on and the required "folder" field empty, the input is
// flagged invalid so it renders with a red outline.
export const RequiredFieldError: Story = {
	args: {
		showErrors: true,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const folder = await canvas.findByRole("textbox", { name: /folder/ });
		await expect(folder).toHaveAttribute("aria-invalid", "true");
	},
};
