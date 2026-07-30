import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fn, userEvent, within } from "storybook/test";
import type {
	TemplateBuilderModule,
	TemplateBuilderModuleVariable,
} from "#/api/typesGenerated";
import { ModuleSettingsStep } from "./ModuleSettingsStep";
import type { SelectedBaseAgent } from "./wizardState";

const baseId = "docker-multi-agent";

const portVariable: TemplateBuilderModuleVariable = {
	name: "port",
	type: "number",
	description: "Port to run code-server on.",
	required: true,
	sensitive: false,
};

const modules: TemplateBuilderModule[] = [
	{
		id: "code-server",
		display_name: "code-server",
		description: "Run VS Code in the browser.",
		category: "IDE",
		icon: "/icon/code.svg",
		version: "1.0.0",
		compatible_os: ["linux"],
		conflicts_with: [],
		variables: [portVariable],
	},
	{
		id: "git-clone",
		display_name: "Git Clone",
		description: "Clone a git repository on workspace start.",
		category: "Source Control",
		icon: "",
		version: "1.0.0",
		compatible_os: ["linux"],
		conflicts_with: [],
		variables: [],
	},
];

const agents: SelectedBaseAgent[] = [
	{ name: "main", displayName: "Primary", default: true },
	{ name: "dev", displayName: "Dev", default: false },
];

const meta: Meta<typeof ModuleSettingsStep> = {
	title: "pages/TemplateBuilder/ModuleSettingsStep",
	component: ModuleSettingsStep,
	args: {
		baseId,
		selectedModuleIds: ["code-server", "git-clone"],
		moduleVariables: {},
		onChangeModuleVariables: fn(),
		onRemoveModule: fn(),
		agents,
		moduleAgents: { "code-server": "main", "git-clone": "main" },
		onChangeModuleAgent: fn(),
	},
	parameters: {
		queries: [
			{
				key: ["templateBuilder", "modules", baseId],
				data: { modules },
			},
		],
	},
};

export default meta;
type Story = StoryObj<typeof ModuleSettingsStep>;

// A multi-agent base renders an agent radio group per selected module, seeded
// to the current selection. Choosing a different agent reports the change for
// that specific module.
export const MultipleAgents: Story = {
	play: async ({ canvasElement, args }) => {
		const canvas = within(canvasElement);

		const primaryRadios = await canvas.findAllByRole("radio", {
			name: "Primary",
		});
		const devRadios = canvas.getAllByRole("radio", { name: "Dev" });
		expect(primaryRadios).toHaveLength(2);
		expect(devRadios).toHaveLength(2);
		expect(primaryRadios[0]).toBeChecked();

		await userEvent.click(devRadios[0]);
		expect(args.onChangeModuleAgent).toHaveBeenCalledWith("code-server", "dev");
	},
};

// A single-agent base hides the per-module agent selector entirely, keeping the
// step focused on variable configuration.
export const SingleAgent: Story = {
	args: {
		agents: [{ name: "main", displayName: "Primary", default: true }],
		moduleAgents: { "code-server": "", "git-clone": "" },
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await canvas.findByText("code-server");
		expect(canvas.queryByRole("radio")).not.toBeInTheDocument();
	},
};
