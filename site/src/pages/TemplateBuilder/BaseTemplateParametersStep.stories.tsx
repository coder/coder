import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn, within } from "storybook/test";
import type { TemplateBuilderBase } from "#/api/typesGenerated";
import { BaseTemplateParametersStep } from "./BaseTemplateParametersStep";

const prerequisites = `## Prerequisites

### Workspace image

The container image determines what tools, languages, and runtimes are
available in the workspace out of the box, so it has a major impact on the
developer experience.

Some options to consider:

- \`codercom/example-base:ubuntu\` (default): minimal and lightweight, but may
  not include many tools developers expect by default
- \`codercom/example-universal:ubuntu\`: catch-all image with many languages
  and tools available, but larger and slower to pull

### Infrastructure

The VM you run Coder on must have a running Docker socket and the \`coder\`
user must be added to the Docker group.
`;

const base: TemplateBuilderBase = {
	id: "docker",
	name: "Docker Containers",
	description: "Provision Docker containers as Coder workspaces",
	icon: "",
	os: "linux",
	prerequisites,
	variables: [
		{
			name: "container_image",
			type: "string",
			description:
				"Container image for workspaces. The image determines which tools and languages are available in the workspace by default. See the template README for guidance on choosing an image.",
			default: { value: "codercom/example-base:ubuntu" },
			required: false,
			sensitive: false,
		},
	],
};

const meta: Meta<typeof BaseTemplateParametersStep> = {
	title: "pages/TemplateBuilder/BaseTemplateParametersStep",
	component: BaseTemplateParametersStep,
	args: {
		baseId: base.id,
		values: {},
		onChangeValues: fn(),
	},
	parameters: {
		queries: [
			{
				key: ["templateBuilder", "bases"],
				data: { bases: [base] },
			},
		],
	},
};

export default meta;
type Story = StoryObj<typeof BaseTemplateParametersStep>;

export const Default: Story = {};

// Verifies that the field description and README body copy are rendered so
// the typography rules in this step have something to style.
export const RendersFieldAndPrerequisites: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await canvas.findByText(/Container image for workspaces/);
		await canvas.findByRole("heading", { name: "Workspace image" });
		await canvas.findByRole("heading", { name: "Infrastructure" });
	},
};
