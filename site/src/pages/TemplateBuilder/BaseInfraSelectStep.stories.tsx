import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fn, within } from "storybook/test";
import type { TemplateBuilderBase } from "#/api/typesGenerated";
import { BaseInfraSelectStep } from "./BaseInfraSelectStep";

function makeBase(
	overrides: Pick<TemplateBuilderBase, "id" | "name" | "description">,
): TemplateBuilderBase {
	return {
		icon: "",
		os: "linux",
		variables: [],
		prerequisites: "",
		...overrides,
	};
}

const bases: TemplateBuilderBase[] = [
	{
		id: "aws-linux",
		name: "AWS EC2 (Linux)",
		description: "Provision AWS EC2 VMs as Coder workspaces",
	},
	{
		id: "aws-windows",
		name: "AWS EC2 (Windows)",
		description: "Provision AWS EC2 VMs as Coder workspaces",
	},
	{
		id: "azure-linux",
		name: "Azure VM (Linux)",
		description: "Provision Azure VMs as Coder workspaces",
	},
	{
		id: "docker",
		name: "Docker Containers",
		description: "Provision Docker containers as Coder workspaces",
	},
].map(makeBase);

const meta: Meta<typeof BaseInfraSelectStep> = {
	title: "pages/TemplateBuilder/BaseInfraSelectStep",
	component: BaseInfraSelectStep,
	args: {
		selectedBaseId: null,
		onSelectBase: fn(),
	},
	parameters: {
		queries: [
			{
				key: ["templateBuilder", "bases"],
				data: { bases },
			},
		],
	},
};

export default meta;
type Story = StoryObj<typeof BaseInfraSelectStep>;

export const Default: Story = {};

export const Selected: Story = {
	args: {
		selectedBaseId: "docker",
	},
};

export const Loading: Story = {
	parameters: {
		queries: [],
	},
};

// Verifies that every base is rendered as a radio option with the expected
// title and description text.
export const RendersAllBases: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const options = await canvas.findAllByRole("radio");
		await expect(options).toHaveLength(bases.length);
		for (const base of bases) {
			await expect(canvas.getByText(base.name)).toBeInTheDocument();
			await expect(
				canvas.getAllByText(base.description).length,
			).toBeGreaterThan(0);
		}
	},
};
