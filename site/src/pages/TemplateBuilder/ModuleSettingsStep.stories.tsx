import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fn, within } from "storybook/test";
import { templateBuilderModules } from "#/api/queries/templateBuilder";
import type { TemplateBuilderModulesResponse } from "#/api/typesGenerated";
import { ModuleSettingsStep } from "./ModuleSettingsStep";

const BASE_ID = "docker";

const modulesResponse: TemplateBuilderModulesResponse = {
	modules: [
		{
			id: "code-server",
			display_name: "code-server",
			description: "Run VS Code in the browser.",
			icon: "/icon/code.svg",
			category: "IDE",
			version: "1.0.0",
			compatible_os: ["linux"],
			conflicts_with: [],
			variables: [
				{
					name: "folder",
					type: "string",
					description:
						"The [folder](https://coder.com/docs) to open in **code-server**.",
					required: true,
					sensitive: false,
				},
				{
					name: "install_prerelease",
					type: "bool",
					description: "Install the `--prerelease` build instead of stable.",
					required: false,
					sensitive: false,
				},
			],
		},
	],
};

const meta: Meta<typeof ModuleSettingsStep> = {
	title: "pages/TemplateBuilder/ModuleSettingsStep",
	component: ModuleSettingsStep,
	parameters: {
		queries: [
			{
				key: templateBuilderModules(BASE_ID).queryKey,
				data: modulesResponse,
			},
		],
	},
	args: {
		baseId: BASE_ID,
		selectedModuleIds: ["code-server"],
		moduleVariables: {},
		onChangeModuleVariables: fn(),
	},
};

export default meta;
type Story = StoryObj<typeof ModuleSettingsStep>;

export const Default: Story = {};

export const MarkdownDescription: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		// The variable description renders Markdown, so the link becomes an anchor.
		const link = await canvas.findByRole("link", { name: /folder/i });
		await expect(link).toHaveAttribute("href", "https://coder.com/docs");
		// Raw Markdown syntax is not shown to the user.
		await expect(canvas.queryByText(/\[folder\]/)).not.toBeInTheDocument();
	},
};
