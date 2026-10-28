import type { Meta, StoryObj } from "@storybook/react-vite";
import { MockFoundationPlanPreview } from "#/testHelpers/templateResources";
import { FoundationPlanPreview } from "./FoundationPlanPreview";

const meta = {
	title: "pages/Foundations/PlanPreview",
	component: FoundationPlanPreview,
	args: { preview: MockFoundationPlanPreview },
} satisfies Meta<typeof FoundationPlanPreview>;
export default meta;
type Story = StoryObj<typeof meta>;
export const ChangedConsumedOutputs: Story = {};
export const Unavailable: Story = {
	args: { preview: { ...MockFoundationPlanPreview, available: false } },
};
export const NoPriorPlan: Story = { args: { preview: undefined } };
export const NoOutputChanges: Story = {
	args: {
		preview: {
			...MockFoundationPlanPreview,
			output_changes: [],
			consumer_references: [],
		},
	},
};
