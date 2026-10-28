import type { Meta, StoryObj } from "@storybook/react-vite";
import { MockFoundation } from "#/testHelpers/templateResources";
import { FoundationImpact } from "./FoundationImpact";

const meta = {
	title: "pages/Foundations/Impact",
	component: FoundationImpact,
	args: { resources: MockFoundation },
} satisfies Meta<typeof FoundationImpact>;
export default meta;
type Story = StoryObj<typeof meta>;
export const NoConsumers: Story = {};
export const Consumers: Story = {
	args: {
		resources: {
			...MockFoundation,
			consumers: { total: 150, running: 100, stopped: 40, unknown: 10 },
		},
	},
};
export const Unattached: Story = {
	args: { resources: { ...MockFoundation, template_id: null } },
};
