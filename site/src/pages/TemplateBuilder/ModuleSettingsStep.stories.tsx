import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fn, userEvent, within } from "storybook/test";
import type {
	TemplateBuilderBase,
	TemplateBuilderModule,
} from "#/api/typesGenerated";
import { ModuleSettingsStep } from "./ModuleSettingsStep";

const baseId = "docker";

const modules: TemplateBuilderModule[] = [
	{
		id: "zed",
		display_name: "Zed",
		description: "Run the Zed editor in your workspace.",
		category: "IDE",
		icon: "/icon/zed.svg",
		version: "1.0.0",
		compatible_os: ["linux"],
		conflicts_with: [],
		variables: [
			{
				name: "agent_name",
				type: "string",
				description: "The agent this module attaches to.",
				required: false,
				sensitive: false,
			},
		],
	},
];

// A multi-agent base: the backend only returns agent_name for these, and the
// step reads the base's agents to build the picker options.
const bases: TemplateBuilderBase[] = [
	{
		id: baseId,
		name: "Docker Containers",
		description: "Run workspaces as Docker containers",
		icon: "/icon/docker.svg",
		os: "linux",
		variables: [],
		prerequisites: "",
		agents: [
			{ name: "main", display_name: "Main", default: true },
			{ name: "gpu", display_name: "GPU", default: false },
		],
	},
];

const meta: Meta<typeof ModuleSettingsStep> = {
	title: "pages/TemplateBuilder/ModuleSettingsStep",
	component: ModuleSettingsStep,
	args: {
		baseId,
		selectedModuleIds: ["zed"],
		moduleVariables: {},
		onChangeModuleVariables: fn(),
		onRemoveModule: fn(),
		registerModuleRef: fn(),
	},
	parameters: {
		queries: [
			{ key: ["templateBuilder", "modules", baseId], data: { modules } },
			{ key: ["templateBuilder", "bases"], data: { bases } },
		],
	},
};

export default meta;
type Story = StoryObj<typeof ModuleSettingsStep>;

// The base's default agent is preselected even though no value is stored yet.
export const DefaultAgentSelected: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(
			await canvas.findByRole("radio", { name: "Main" }),
		).toBeChecked();
		await expect(canvas.getByRole("radio", { name: "GPU" })).not.toBeChecked();
	},
};

// Choosing another agent stores its name as the agent_name variable.
export const SelectingAgentUpdatesVariable: Story = {
	play: async ({ canvasElement, args }) => {
		const canvas = within(canvasElement);
		await userEvent.click(await canvas.findByRole("radio", { name: "GPU" }));
		await expect(args.onChangeModuleVariables).toHaveBeenCalledWith("zed", {
			agent_name: "gpu",
		});
	},
};
