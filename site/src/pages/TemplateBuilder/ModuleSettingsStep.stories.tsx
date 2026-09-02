import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fn, userEvent, within } from "storybook/test";
import type { TemplateBuilderModule } from "#/api/typesGenerated";
import { ModuleSettingsStep } from "./ModuleSettingsStep";
import type { SelectedBaseAgent } from "./wizardState";

const baseId = "docker";

const codeServer: TemplateBuilderModule = {
	id: "code-server",
	display_name: "code-server",
	description: "",
	icon: "",
	category: "IDE",
	version: "1.0.0",
	compatible_os: ["linux"],
	conflicts_with: [],
	variables: [],
};

const twoAgents: SelectedBaseAgent[] = [
	{ name: "main", displayName: "Main", default: true },
	{ name: "gpu", displayName: "GPU", default: false },
];

const oneAgent: SelectedBaseAgent[] = [
	{ name: "main", displayName: "Main", default: true },
];

const meta: Meta<typeof ModuleSettingsStep> = {
	title: "pages/TemplateBuilder/ModuleSettingsStep",
	component: ModuleSettingsStep,
	args: {
		baseId,
		selectedModuleIds: ["code-server"],
		moduleVariables: {},
		moduleAgents: {},
		agents: oneAgent,
		onChangeModuleVariables: fn(),
		onChangeModuleAgent: fn(),
		onRemoveModule: fn(),
		registerModuleRef: fn(),
	},
	parameters: {
		queries: [
			{
				key: ["templateBuilder", "modules", baseId],
				data: { modules: [codeServer] },
			},
		],
	},
};

export default meta;
type Story = StoryObj<typeof ModuleSettingsStep>;

// A single-agent base shows no agent picker: there is nothing to choose.
export const SingleAgent: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await canvas.findByText("code-server");
		await expect(canvas.queryByText("Agent")).not.toBeInTheDocument();
	},
};

// A multi-agent base shows the picker with the base default preselected.
export const MultipleAgents: Story = {
	args: { agents: twoAgents },
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await canvas.findByText("Agent");
		await expect(canvas.getByRole("radio", { name: /Main/ })).toBeChecked();
	},
};

// Picking a different agent reports the choice to the caller.
export const ChangeAgent: Story = {
	args: { agents: twoAgents },
	play: async ({ canvasElement, args }) => {
		const canvas = within(canvasElement);
		await canvas.findByText("Agent");
		await userEvent.click(canvas.getByRole("radio", { name: /GPU/ }));
		await expect(args.onChangeModuleAgent).toHaveBeenCalledWith(
			"code-server",
			"gpu",
		);
	},
};
